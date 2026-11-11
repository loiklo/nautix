// Package api implements the JSON HTTP API.
package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-sql-driver/mysql"

	"nautix/internal/auth"
	"nautix/internal/hub"
)

type Server struct {
	DB   *sql.DB
	Hub  *hub.Hub
	Auth *auth.Auth
}

// Routes registers every API endpoint on the mux.
func (s *Server) Routes(mux *http.ServeMux) {
	user, admin := s.Auth.Require, s.Auth.RequireAdmin

	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("POST /api/logout", s.logout)
	mux.HandleFunc("GET /ws", user(s.Hub.Serve))

	mux.HandleFunc("GET /api/state", user(s.state))
	mux.HandleFunc("PUT /api/me/section", user(s.setLastSection))
	mux.HandleFunc("PUT /api/statuses", user(s.setStatus))
	mux.HandleFunc("GET /api/journal", user(s.journal))

	mux.HandleFunc("POST /api/sections", user(s.createSection))
	mux.HandleFunc("PUT /api/sections/{id}", user(s.rename("sections", "Une section porte déjà ce nom.")))
	mux.HandleFunc("DELETE /api/sections/{id}", user(s.remove("sections")))

	mux.HandleFunc("POST /api/students", user(s.createStudent))
	mux.HandleFunc("PUT /api/students/{id}", user(s.rename("students", duplicateStudent)))
	mux.HandleFunc("DELETE /api/students/{id}", user(s.remove("students")))

	mux.HandleFunc("POST /api/groups", user(s.createGroup))
	mux.HandleFunc("PUT /api/groups/{id}", user(s.rename("skill_groups", "")))
	mux.HandleFunc("DELETE /api/groups/{id}", user(s.remove("skill_groups")))
	mux.HandleFunc("POST /api/groups/{id}/move", user(s.move("skill_groups", "section_id")))

	mux.HandleFunc("POST /api/skills", user(s.createSkill))
	mux.HandleFunc("PUT /api/skills/{id}", user(s.rename("skills", "")))
	mux.HandleFunc("DELETE /api/skills/{id}", user(s.remove("skills")))
	mux.HandleFunc("POST /api/skills/{id}/move", user(s.move("skills", "group_id")))

	mux.HandleFunc("GET /api/admin/instructors", admin(s.listInstructors))
	mux.HandleFunc("POST /api/admin/instructors", admin(s.createInstructor))
	mux.HandleFunc("PUT /api/admin/instructors/{id}", admin(s.updateInstructor))
	mux.HandleFunc("DELETE /api/admin/instructors/{id}", admin(s.deleteInstructor))
	mux.HandleFunc("GET /api/admin/sessions", admin(s.listSessions))
	mux.HandleFunc("DELETE /api/admin/sessions/{id}", admin(s.deleteSession))
	mux.HandleFunc("DELETE /api/admin/students", admin(s.deleteAllStudents))
	mux.HandleFunc("DELETE /api/admin/journal", admin(s.clearJournal))
}

// ---- helpers ----

const duplicateStudent = "Un élève porte déjà ce nom dans cette section."

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(v)
}

func ok(w http.ResponseWriter) {
	writeJSON(w, map[string]bool{"ok": true})
}

func fail(w http.ResponseWriter, code int, msg string) {
	http.Error(w, msg, code)
}

// readJSON decodes the request body. Requiring a JSON content type also
// blocks cross-site form posts.
func readJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		fail(w, http.StatusUnsupportedMediaType, "Requête invalide.")
		return false
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(dst); err != nil {
		fail(w, http.StatusBadRequest, "Requête invalide.")
		return false
	}
	return true
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		fail(w, http.StatusBadRequest, "Identifiant invalide.")
		return 0, false
	}
	return id, true
}

// cleanName trims a name and checks its length.
func cleanName(w http.ResponseWriter, name string, max int) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > max {
		fail(w, http.StatusBadRequest, fmt.Sprintf("Le nom est obligatoire (%d caractères maximum).", max))
		return "", false
	}
	return name, true
}

// dbFail converts a database error into an HTTP error.
func dbFail(w http.ResponseWriter, err error, duplicateMsg string) {
	var me *mysql.MySQLError
	switch {
	case errors.Is(err, sql.ErrNoRows):
		fail(w, http.StatusNotFound, "Élément introuvable (peut-être déjà supprimé).")
	case errors.As(err, &me) && me.Number == 1062:
		if duplicateMsg == "" {
			duplicateMsg = "Ce nom existe déjà."
		}
		fail(w, http.StatusConflict, duplicateMsg)
	case errors.As(err, &me) && me.Number == 1452:
		fail(w, http.StatusBadRequest, "Élément parent introuvable (peut-être supprimé).")
	default:
		log.Printf("database error: %v", err)
		fail(w, http.StatusInternalServerError, "Erreur interne.")
	}
}

// ---- session ----

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct{ Name, Password string }
	if !readJSON(w, r, &in) {
		return
	}
	switch err := s.Auth.Login(w, r, strings.TrimSpace(in.Name), in.Password); {
	case errors.Is(err, auth.ErrThrottled):
		fail(w, http.StatusTooManyRequests, "Trop de tentatives. Réessayez dans une minute.")
	case errors.Is(err, auth.ErrBadCredentials):
		fail(w, http.StatusUnauthorized, "Nom ou mot de passe incorrect.")
	case err != nil:
		dbFail(w, err, "")
	default:
		s.Hub.Refresh() // the admin session list changed
		ok(w)
	}
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	s.Auth.Logout(w, r)
	s.Hub.Refresh()
	ok(w)
}

// ---- state ----

type named struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type child struct {
	ID       int64  `json:"id"`
	ParentID int64  `json:"parent_id"`
	Name     string `json:"name"`
}

// state returns everything the main views need in a single response.
func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r)
	out := struct {
		Me struct {
			ID            int64  `json:"id"`
			Name          string `json:"name"`
			IsAdmin       bool   `json:"is_admin"`
			LastSectionID *int64 `json:"last_section_id"`
		} `json:"me"`
		Sections []named    `json:"sections"`
		Groups   []child    `json:"groups"`   // parent: section, in display order
		Skills   []child    `json:"skills"`   // parent: group, in display order
		Students []child    `json:"students"` // parent: section
		Statuses [][3]int64 `json:"statuses"` // [student, skill, status], NA omitted
	}{Sections: []named{}, Groups: []child{}, Skills: []child{}, Students: []child{}, Statuses: [][3]int64{}}
	out.Me.ID, out.Me.Name, out.Me.IsAdmin, out.Me.LastSectionID = u.ID, u.Name, u.IsAdmin, u.LastSectionID

	rows, err := s.DB.Query("SELECT id, name FROM sections ORDER BY name")
	if err != nil {
		dbFail(w, err, "")
		return
	}
	for rows.Next() {
		var n named
		rows.Scan(&n.ID, &n.Name)
		out.Sections = append(out.Sections, n)
	}
	rows.Close()

	children := func(query string, dst *[]child) error {
		rows, err := s.DB.Query(query)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c child
			if err := rows.Scan(&c.ID, &c.ParentID, &c.Name); err != nil {
				return err
			}
			*dst = append(*dst, c)
		}
		return rows.Err()
	}
	if err := errors.Join(
		children("SELECT id, section_id, name FROM skill_groups ORDER BY position, id", &out.Groups),
		children("SELECT id, group_id, name FROM skills ORDER BY position, id", &out.Skills),
		children("SELECT id, section_id, name FROM students", &out.Students),
	); err != nil {
		dbFail(w, err, "")
		return
	}

	rows, err = s.DB.Query("SELECT student_id, skill_id, status FROM statuses WHERE status > 0")
	if err != nil {
		dbFail(w, err, "")
		return
	}
	for rows.Next() {
		var t [3]int64
		rows.Scan(&t[0], &t[1], &t[2])
		out.Statuses = append(out.Statuses, t)
	}
	rows.Close()
	writeJSON(w, out)
}

func (s *Server) setLastSection(w http.ResponseWriter, r *http.Request) {
	var in struct {
		SectionID int64 `json:"section_id"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if _, err := s.DB.Exec("UPDATE instructors SET last_section_id = ? WHERE id = ?", in.SectionID, auth.UserFrom(r).ID); err != nil {
		dbFail(w, err, "")
		return
	}
	ok(w)
}

// ---- statuses and journal ----

var statusLabels = [...]string{"NA", "ECA", "A", "M"}

// setStatus stores one skill status for one student, logs the change and
// pushes it to every client.
func (s *Server) setStatus(w http.ResponseWriter, r *http.Request) {
	var in struct {
		StudentID int64 `json:"student_id"`
		SkillID   int64 `json:"skill_id"`
		Status    int   `json:"status"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Status < 0 || in.Status >= len(statusLabels) {
		fail(w, http.StatusBadRequest, "Statut invalide.")
		return
	}

	tx, err := s.DB.Begin()
	if err != nil {
		dbFail(w, err, "")
		return
	}
	defer tx.Rollback()

	// The join guarantees the student and the skill belong to the same section.
	var student, skill, section string
	err = tx.QueryRow(`SELECT st.name, sk.name, se.name
		FROM students st
		JOIN sections se ON se.id = st.section_id
		JOIN skill_groups g ON g.section_id = se.id
		JOIN skills sk ON sk.group_id = g.id
		WHERE st.id = ? AND sk.id = ?`, in.StudentID, in.SkillID).Scan(&student, &skill, &section)
	if err != nil {
		dbFail(w, err, "")
		return
	}

	old := 0
	err = tx.QueryRow("SELECT status FROM statuses WHERE student_id = ? AND skill_id = ? FOR UPDATE", in.StudentID, in.SkillID).Scan(&old)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		dbFail(w, err, "")
		return
	}
	if old == in.Status {
		ok(w)
		return
	}

	if _, err := tx.Exec(`INSERT INTO statuses (student_id, skill_id, status) VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE status = VALUES(status)`, in.StudentID, in.SkillID, in.Status); err != nil {
		dbFail(w, err, "")
		return
	}
	action := fmt.Sprintf("%s — %s : %s : %s → %s", section, student, skill, statusLabels[old], statusLabels[in.Status])
	if _, err := tx.Exec("INSERT INTO journal (created_at, instructor_name, action) VALUES (?, ?, ?)",
		time.Now().UTC(), auth.UserFrom(r).Name, action); err != nil {
		dbFail(w, err, "")
		return
	}
	if err := tx.Commit(); err != nil {
		dbFail(w, err, "")
		return
	}

	s.Hub.Broadcast(map[string]any{"type": "status", "student_id": in.StudentID, "skill_id": in.SkillID, "status": in.Status})
	ok(w)
}

// journal returns the 100 most recent actions.
func (s *Server) journal(w http.ResponseWriter, r *http.Request) {
	type entry struct {
		At         time.Time `json:"at"`
		Instructor string    `json:"instructor"`
		Action     string    `json:"action"`
	}
	out := []entry{}
	rows, err := s.DB.Query("SELECT created_at, instructor_name, action FROM journal ORDER BY id DESC LIMIT 100")
	if err != nil {
		dbFail(w, err, "")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var e entry
		rows.Scan(&e.At, &e.Instructor, &e.Action)
		out = append(out, e)
	}
	writeJSON(w, out)
}
