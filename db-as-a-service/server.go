package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
)

// Server is an HTTP handler that routes requests to the database manager.
// Use [NewServer] to construct one.
type Server struct {
	manager *DBManager
	mux     *http.ServeMux
}

// NewServer constructs a Server wired to manager and registers all routes.
func NewServer(manager *DBManager) *Server {
	s := &Server{
		manager: manager,
		mux:     http.NewServeMux(),
	}
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("POST /databases/{name}", s.handleCreateDB)
	s.mux.HandleFunc("GET /databases", s.handleListDBs)
	s.mux.HandleFunc("GET /databases/{name}/stats", s.handleGetDBStats)
	s.mux.HandleFunc("DELETE /databases/{name}", s.handleDeleteDB)
	s.mux.HandleFunc("PUT /databases/{name}/keys/{key}", s.handlePutKey)
	s.mux.HandleFunc("GET /databases/{name}/keys/{key}", s.handleGetKey)
	return s
}

// ServeHTTP implements [http.Handler].
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// handleHealth returns {"status": "ok"}.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleCreateDB handles POST /databases/{name}.
// Creates a new named database and returns 201 {"name": "<name>"}.
// Returns 409 if the database already exists.
func (s *Server) handleCreateDB(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.manager.Create(name); err != nil {
		if errors.Is(err, ErrAlreadyExists) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"name": name})
}

// handleListDBs handles GET /databases.
// Returns 200 {"databases": ["name", ...]}.
func (s *Server) handleListDBs(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string][]string{"databases": s.manager.List()})
}

// handleGetDBStats handles GET /databases/{name}/stats.
// Returns 200 {"name": "<name>", "sstable_count": N}.
// Returns 404 if the database does not exist.
func (s *Server) handleGetDBStats(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	db, err := s.manager.Get(name)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	stats := db.Stats()
	writeJSON(w, http.StatusOK, map[string]any{
		"name":          name,
		"sstable_count": stats.SSTableCount,
	})
}

// handleDeleteDB handles DELETE /databases/{name}.
// Deletes the database and its data. Returns 200 {"name": "<name>"}.
// Returns 404 if the database does not exist.
func (s *Server) handleDeleteDB(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.manager.Delete(name); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"name": name})
}

// handlePutKey handles PUT /databases/{name}/keys/{key}.
// Reads {"value": "..."} from the request body and writes it to the database.
// Returns 200 {"key": "<key>", "value": "<value>"}.
// Returns 404 if the database does not exist, 400 on malformed body.
func (s *Server) handlePutKey(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	key := r.PathValue("key")

	db, err := s.manager.Get(name)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var body struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := db.Put(key, body.Value); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"key": key, "value": body.Value})
}

// handleGetKey handles GET /databases/{name}/keys/{key}.
// Returns 200 {"key": "<key>", "value": "<value>"} or 404 if not found.
func (s *Server) handleGetKey(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	key := r.PathValue("key")

	db, err := s.manager.Get(name)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	value, ok, err := db.Get(key)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "key not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"key": key, "value": value})
}

// writeJSON serialises v as JSON and writes it with the given HTTP status code.
// Content-Type is set to application/json.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON: %v", err)
	}
}

// writeError writes {"error": "<msg>"} with the given HTTP status code.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
