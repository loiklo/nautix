package api

import (
	"database/sql"
	"errors"
	"net/http"
)

// Table and column names passed to the generic handlers below are constants
// from Routes, never user input.

func (s *Server) createSection(w http.ResponseWriter, r *http.Request) {
	var in struct{ Name string }
	if !readJSON(w, r, &in) {
		return
	}
	name, valid := cleanName(w, in.Name, 100)
	if !valid {
		return
	}
	if _, err := s.DB.Exec("INSERT INTO sections (name) VALUES (?)", name); err != nil {
		dbFail(w, err, "Une section porte déjà ce nom.")
		return
	}
	s.Hub.Refresh()
	ok(w)
}

func (s *Server) createStudent(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name      string
		SectionID int64 `json:"section_id"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	name, valid := cleanName(w, in.Name, 100)
	if !valid {
		return
	}
	if _, err := s.DB.Exec("INSERT INTO students (section_id, name) VALUES (?, ?)", in.SectionID, name); err != nil {
		dbFail(w, err, duplicateStudent)
		return
	}
	s.Hub.Refresh()
	ok(w)
}

func (s *Server) createGroup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name      string
		SectionID int64 `json:"section_id"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	name, valid := cleanName(w, in.Name, 100)
	if !valid {
		return
	}
	if _, err := s.DB.Exec(`INSERT INTO skill_groups (section_id, name, position)
		SELECT ?, ?, COALESCE(MAX(position), 0) + 1 FROM skill_groups WHERE section_id = ?`,
		in.SectionID, name, in.SectionID); err != nil {
		dbFail(w, err, "")
		return
	}
	s.Hub.Refresh()
	ok(w)
}

func (s *Server) createSkill(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name    string
		GroupID int64 `json:"group_id"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	name, valid := cleanName(w, in.Name, 200)
	if !valid {
		return
	}
	if _, err := s.DB.Exec(`INSERT INTO skills (group_id, name, position)
		SELECT ?, ?, COALESCE(MAX(position), 0) + 1 FROM skills WHERE group_id = ?`,
		in.GroupID, name, in.GroupID); err != nil {
		dbFail(w, err, "")
		return
	}
	s.Hub.Refresh()
	ok(w)
}

// rename changes only the name of a row: a student can never change section,
// nor a group or a skill change parent.
func (s *Server) rename(table, duplicateMsg string) http.HandlerFunc {
	max := 100
	if table == "skills" {
		max = 200
	}
	return func(w http.ResponseWriter, r *http.Request) {
		id, valid := pathID(w, r)
		if !valid {
			return
		}
		var in struct{ Name string }
		if !readJSON(w, r, &in) {
			return
		}
		name, valid := cleanName(w, in.Name, max)
		if !valid {
			return
		}
		if _, err := s.DB.Exec("UPDATE "+table+" SET name = ? WHERE id = ?", name, id); err != nil {
			dbFail(w, err, duplicateMsg)
			return
		}
		s.Hub.Refresh()
		ok(w)
	}
}

// remove deletes a row; foreign keys cascade to everything that depends on it.
func (s *Server) remove(table string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, valid := pathID(w, r)
		if !valid {
			return
		}
		if _, err := s.DB.Exec("DELETE FROM "+table+" WHERE id = ?", id); err != nil {
			dbFail(w, err, "")
			return
		}
		s.Hub.Refresh()
		ok(w)
	}
}

// move swaps a row with its previous or next sibling in the manual order.
func (s *Server) move(table, parentCol string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, valid := pathID(w, r)
		if !valid {
			return
		}
		var in struct{ Direction string }
		if !readJSON(w, r, &in) {
			return
		}
		cmp, order := "<", "DESC"
		switch in.Direction {
		case "up":
		case "down":
			cmp, order = ">", "ASC"
		default:
			fail(w, http.StatusBadRequest, "Direction invalide.")
			return
		}

		tx, err := s.DB.Begin()
		if err != nil {
			dbFail(w, err, "")
			return
		}
		defer tx.Rollback()

		var parent, pos int64
		if err := tx.QueryRow("SELECT "+parentCol+", position FROM "+table+" WHERE id = ? FOR UPDATE", id).Scan(&parent, &pos); err != nil {
			dbFail(w, err, "")
			return
		}
		var otherID, otherPos int64
		err = tx.QueryRow("SELECT id, position FROM "+table+" WHERE "+parentCol+" = ? AND position "+cmp+" ? ORDER BY position "+order+" LIMIT 1 FOR UPDATE",
			parent, pos).Scan(&otherID, &otherPos)
		if errors.Is(err, sql.ErrNoRows) {
			ok(w) // already first or last
			return
		}
		if err != nil {
			dbFail(w, err, "")
			return
		}
		if _, err := tx.Exec("UPDATE "+table+" SET position = ? WHERE id = ?", otherPos, id); err != nil {
			dbFail(w, err, "")
			return
		}
		if _, err := tx.Exec("UPDATE "+table+" SET position = ? WHERE id = ?", pos, otherID); err != nil {
			dbFail(w, err, "")
			return
		}
		if err := tx.Commit(); err != nil {
			dbFail(w, err, "")
			return
		}
		s.Hub.Refresh()
		ok(w)
	}
}
