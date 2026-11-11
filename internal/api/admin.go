package api

import (
	"net/http"
	"time"

	"nautix/internal/auth"
)

// rootAdmin is the built-in administrator: only this account can change its
// own password, and it can never be renamed, demoted or deleted.
const rootAdmin = "Alex"

func (s *Server) listInstructors(w http.ResponseWriter, r *http.Request) {
	type instructor struct {
		ID      int64  `json:"id"`
		Name    string `json:"name"`
		IsAdmin bool   `json:"is_admin"`
	}
	out := []instructor{}
	rows, err := s.DB.Query("SELECT id, name, is_admin FROM instructors ORDER BY name")
	if err != nil {
		dbFail(w, err, "")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var i instructor
		rows.Scan(&i.ID, &i.Name, &i.IsAdmin)
		out = append(out, i)
	}
	writeJSON(w, out)
}

func (s *Server) createInstructor(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name     string
		Password string
		IsAdmin  bool `json:"is_admin"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	name, valid := cleanName(w, in.Name, 100)
	if !valid {
		return
	}
	if in.Password == "" {
		fail(w, http.StatusBadRequest, "Le mot de passe est obligatoire.")
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		fail(w, http.StatusBadRequest, "Mot de passe invalide (72 caractères maximum).")
		return
	}
	if _, err := s.DB.Exec("INSERT INTO instructors (name, password_hash, is_admin) VALUES (?, ?, ?)", name, hash, in.IsAdmin); err != nil {
		dbFail(w, err, "Un moniteur porte déjà ce nom.")
		return
	}
	s.Hub.Refresh()
	ok(w)
}

// updateInstructor changes any of the name, password and admin flag; absent
// fields are left untouched.
func (s *Server) updateInstructor(w http.ResponseWriter, r *http.Request) {
	id, valid := pathID(w, r)
	if !valid {
		return
	}
	var in struct {
		Name     *string
		Password *string
		IsAdmin  *bool `json:"is_admin"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	me := auth.UserFrom(r)

	var current string
	if err := s.DB.QueryRow("SELECT name FROM instructors WHERE id = ?", id).Scan(&current); err != nil {
		dbFail(w, err, "")
		return
	}

	if current == rootAdmin {
		if in.Name != nil {
			fail(w, http.StatusForbidden, "Le compte Alex ne peut pas être renommé.")
			return
		}
		if in.IsAdmin != nil && !*in.IsAdmin {
			fail(w, http.StatusForbidden, "Les droits administrateur du compte Alex ne peuvent pas être retirés.")
			return
		}
		if in.Password != nil && me.Name != rootAdmin {
			fail(w, http.StatusForbidden, "Seul Alex peut modifier son mot de passe.")
			return
		}
	}

	if in.Name != nil {
		name, valid := cleanName(w, *in.Name, 100)
		if !valid {
			return
		}
		if _, err := s.DB.Exec("UPDATE instructors SET name = ? WHERE id = ?", name, id); err != nil {
			dbFail(w, err, "Un moniteur porte déjà ce nom.")
			return
		}
	}
	if in.IsAdmin != nil {
		if _, err := s.DB.Exec("UPDATE instructors SET is_admin = ? WHERE id = ?", *in.IsAdmin, id); err != nil {
			dbFail(w, err, "")
			return
		}
	}
	if in.Password != nil {
		if *in.Password == "" {
			fail(w, http.StatusBadRequest, "Le mot de passe est obligatoire.")
			return
		}
		hash, err := auth.HashPassword(*in.Password)
		if err != nil {
			fail(w, http.StatusBadRequest, "Mot de passe invalide (72 caractères maximum).")
			return
		}
		if _, err := s.DB.Exec("UPDATE instructors SET password_hash = ? WHERE id = ?", hash, id); err != nil {
			dbFail(w, err, "")
			return
		}
		// A new password closes every other session of that instructor.
		if _, err := s.DB.Exec("DELETE FROM sessions WHERE instructor_id = ? AND id <> ?", id, me.SessionID); err != nil {
			dbFail(w, err, "")
			return
		}
		s.Hub.KickAll()
	}
	s.Hub.Refresh()
	ok(w)
}

func (s *Server) deleteInstructor(w http.ResponseWriter, r *http.Request) {
	id, valid := pathID(w, r)
	if !valid {
		return
	}
	var name string
	if err := s.DB.QueryRow("SELECT name FROM instructors WHERE id = ?", id).Scan(&name); err != nil {
		dbFail(w, err, "")
		return
	}
	if name == rootAdmin {
		fail(w, http.StatusForbidden, "Le compte Alex ne peut pas être supprimé.")
		return
	}
	if _, err := s.DB.Exec("DELETE FROM instructors WHERE id = ?", id); err != nil { // sessions cascade
		dbFail(w, err, "")
		return
	}
	s.Hub.KickAll()
	s.Hub.Refresh()
	ok(w)
}

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request) {
	type session struct {
		ID         int64     `json:"id"`
		Instructor string    `json:"instructor"`
		CreatedAt  time.Time `json:"created_at"`
		LastUsedAt time.Time `json:"last_used_at"`
		Current    bool      `json:"current"`
	}
	out := []session{}
	rows, err := s.DB.Query(`SELECT s.id, i.name, s.created_at, s.last_used_at
		FROM sessions s JOIN instructors i ON i.id = s.instructor_id ORDER BY s.last_used_at DESC, s.id DESC`)
	if err != nil {
		dbFail(w, err, "")
		return
	}
	defer rows.Close()
	current := auth.UserFrom(r).SessionID
	for rows.Next() {
		var e session
		rows.Scan(&e.ID, &e.Instructor, &e.CreatedAt, &e.LastUsedAt)
		e.Current = e.ID == current
		out = append(out, e)
	}
	writeJSON(w, out)
}

func (s *Server) deleteSession(w http.ResponseWriter, r *http.Request) {
	id, valid := pathID(w, r)
	if !valid {
		return
	}
	if _, err := s.DB.Exec("DELETE FROM sessions WHERE id = ?", id); err != nil {
		dbFail(w, err, "")
		return
	}
	s.Hub.KickAll()
	s.Hub.Refresh()
	ok(w)
}

func (s *Server) deleteAllStudents(w http.ResponseWriter, r *http.Request) {
	if _, err := s.DB.Exec("DELETE FROM students"); err != nil { // statuses cascade
		dbFail(w, err, "")
		return
	}
	s.Hub.Refresh()
	ok(w)
}

func (s *Server) clearJournal(w http.ResponseWriter, r *http.Request) {
	if _, err := s.DB.Exec("DELETE FROM journal"); err != nil {
		dbFail(w, err, "")
		return
	}
	s.Hub.Refresh()
	ok(w)
}
