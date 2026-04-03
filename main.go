package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	storageFile   = "messages.json"
	staticDir     = "static"
	serverAddress = ":8080"
	logFile       = "server.log"
)

type Message struct {
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"`
}

type Storage struct {
	mu        sync.RWMutex
	messages  []Message
	filePath  string
}

func NewStorage(filePath string) *Storage {
	s := &Storage{
		filePath: filePath,
	}
	if err := s.load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("error loading storage from file %s: %v", filePath, err)
	}
	return s
}

func (s *Storage) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.Open(s.filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return err
	}

	if len(data) == 0 {
		s.messages = nil
		return nil
	}

	var msgs []Message
	if err := json.Unmarshal(data, &msgs); err != nil {
		return err
	}
	s.messages = msgs
	return nil
}

func (s *Storage) save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tmp := s.filePath + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(s.messages); err != nil {
		f.Close()
		return err
	}

	if err := f.Close(); err != nil {
		return err
	}

	return os.Rename(tmp, s.filePath)
}

func (s *Storage) Add(text string) (Message, error) {
	if text == "" {
		return Message{}, errors.New("empty message")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	msg := Message{
		Text:      text,
		Timestamp: time.Now().UTC(),
	}
	s.messages = append(s.messages, msg)
	if err := s.saveUnlocked(); err != nil {
		return Message{}, err
	}
	return msg, nil
}

// saveUnlocked assumes that s.mu is already locked for writing.
func (s *Storage) saveUnlocked() error {
	tmp := s.filePath + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(s.messages); err != nil {
		f.Close()
		return err
	}

	if err := f.Close(); err != nil {
		return err
	}

	return os.Rename(tmp, s.filePath)
}

func (s *Storage) All() []Message {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Message, len(s.messages))
	copy(out, s.messages)
	return out
}

type server struct {
	storage *Storage
}

var loggerFile *os.File

func initLogger() {
	absLogPath, err := filepath.Abs(logFile)
	if err != nil {
		// Если путь не удалось вычислить, оставляем логирование в stdout.
		log.Printf("initLogger: cannot resolve log file path: %v", err)
		return
	}

	f, err := os.OpenFile(absLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		// Если файл открыть нельзя, логируем только в stdout.
		log.Printf("initLogger: cannot open log file %s: %v", absLogPath, err)
		return
	}
	loggerFile = f // удерживаем дескриптор на время жизни процесса

	log.SetOutput(io.MultiWriter(os.Stdout, f))
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
}

func (s *server) logRequest(r *http.Request) {
	ip := clientIP(r)
	log.Printf("request: method=%s path=%s ip=%s", r.Method, r.URL.Path, ip)
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("error encoding json response: %v", err)
	}
}

func (s *server) handlePostMessage(w http.ResponseWriter, r *http.Request) {
	s.logRequest(r)
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	type reqBody struct {
		Text string `json:"text"`
	}

	var body reqBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		log.Printf("error decoding request body: %v", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}

	msg, err := s.storage.Add(body.Text)
	if err != nil {
		log.Printf("error adding message: %v", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"status":  "ok",
		"message": msg,
	})
}

func (s *server) handleGetMessages(w http.ResponseWriter, r *http.Request) {
	s.logRequest(r)
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	msgs := s.storage.All()
	writeJSON(w, http.StatusOK, msgs)
}

func main() {
	initLogger()
	log.Println("starting Go message server...")

	absStoragePath, err := filepath.Abs(storageFile)
	if err != nil {
		log.Fatalf("cannot resolve storage file path: %v", err)
	}

	st := NewStorage(absStoragePath)
	s := &server{storage: st}

	mux := http.NewServeMux()

	mux.HandleFunc("/message", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			s.handlePostMessage(w, r)
		case http.MethodGet:
			s.handleGetMessages(w, r)
		default:
			s.logRequest(r)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
	})

	fileServer := http.FileServer(http.Dir(staticDir))
	mux.Handle("/", fileServer)

	server := &http.Server{
		Addr:         serverAddress,
		Handler:      loggingMiddleware(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("server listening on %s", serverAddress)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server error: %v", err)
	}
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ip := clientIP(r)
		log.Printf("http request: method=%s path=%s ip=%s", r.Method, r.URL.Path, ip)
		next.ServeHTTP(w, r)
		log.Printf("http response: method=%s path=%s ip=%s duration=%s", r.Method, r.URL.Path, ip, time.Since(start))
	})
}

