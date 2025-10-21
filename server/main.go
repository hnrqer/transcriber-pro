package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	port          = "8456"
	uploadDir     = "/tmp/transcriber-uploads"
	maxUploadSize = 20 * 1024 * 1024 * 1024 // 20GB limit
)

var Version = "dev"

var engine *TranscriptionEngine

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println(Version)
		return
	}

	var err error
	engine, err = NewTranscriptionEngine()
	if err != nil {
		log.Fatalf("Failed to initialize transcription engine: %v", err)
	}
	defer engine.Close()

	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		log.Fatalf("Failed to create upload directory: %v", err)
	}

	// Try to find static directory in multiple locations
	staticDir := findStaticDir()
	if staticDir == "" {
		log.Fatal("Static directory not found. Tried: ./static, /opt/homebrew/share/transcriber-pro/static, /usr/local/share/transcriber-pro/static")
	}
	absStaticDir := staticDir

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Disable caching for static files to avoid stale content
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")

		if r.URL.Path == "/" {
			http.ServeFile(w, r, filepath.Join(absStaticDir, "index.html"))
		} else {
			http.FileServer(http.Dir(absStaticDir)).ServeHTTP(w, r)
		}
	})

	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/version", handleVersion)
	http.HandleFunc("/transcribe", handleTranscribe)
	http.HandleFunc("/progress/", handleProgress)
	http.HandleFunc("/queue", handleQueue)
	http.HandleFunc("/clear-completed", handleClearCompleted)
	http.HandleFunc("/clear-all", handleClearAll)
	http.HandleFunc("/cancel-job/", handleCancelJob)
	http.HandleFunc("/kill-job/", handleKillJob)

	serverURL := fmt.Sprintf("http://localhost:%s", port)

	go func() {
		time.Sleep(1500 * time.Millisecond)
		openBrowser(serverURL)
	}()

	fmt.Println("========================================")
	fmt.Println("Transcriber Pro - Companion")
	fmt.Println("========================================")
	fmt.Println()
	fmt.Printf("Server running at %s\n", serverURL)
	fmt.Println()
	fmt.Println("The companion will automatically download the Whisper model on first run (~3GB).")
	fmt.Println("This may take several minutes depending on your internet connection.")
	fmt.Println()
	fmt.Println("Press Ctrl+C to stop the server.")
	fmt.Println()

	srv := &http.Server{
		Addr: ":" + port,
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)

	go func() {
		<-sigChan
		fmt.Println("\nShutting down...")
		srv.Shutdown(context.Background())
	}()

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "Local Server"
	}
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"device": hostname,
	})
}

func handleVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"version": Version})
}

func sendJSONError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func handleTranscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

	// Parse multipart form with 32MB memory limit, rest goes to disk
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		log.Printf("ParseMultipartForm error: %v", err)
		sendJSONError(w, fmt.Sprintf("Failed to parse upload: %v", err), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("audio")
	if err != nil {
		log.Printf("FormFile error: %v", err)
		sendJSONError(w, "No audio file provided", http.StatusBadRequest)
		return
	}
	defer file.Close()

	language := r.FormValue("language")
	if language == "" {
		language = "auto"
	}

	jobID := uuid.New().String()
	fileName := header.Filename
	ext := filepath.Ext(fileName)
	audioPath := filepath.Join(uploadDir, jobID+ext)

	dst, err := os.Create(audioPath)
	if err != nil {
		sendJSONError(w, "Failed to save file", http.StatusInternalServerError)
		return
	}

	_, err = io.Copy(dst, file)
	dst.Close()
	if err != nil {
		os.Remove(audioPath)
		sendJSONError(w, "Failed to save file", http.StatusInternalServerError)
		return
	}

	// Create job and add to queue - queue processor will handle transcription
	engine.CreateJob(jobID, fileName, audioPath, language)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"job_id": jobID,
		"status": string(StatusQueued),
	})
}

func handleProgress(w http.ResponseWriter, r *http.Request) {
	jobID := strings.TrimPrefix(r.URL.Path, "/progress/")
	if jobID == "" {
		sendJSONError(w, "Job ID required", http.StatusBadRequest)
		return
	}

	job := engine.GetJob(jobID)
	if job == nil {
		sendJSONError(w, "Job not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	response := map[string]interface{}{
		"status":   string(job.Status),
		"progress": job.Progress,
		"message":  job.Message,
		"eta":      job.ETA,
	}

	if job.Status == StatusCompleted && job.Result != nil {
		response["result"] = job.Result
	}

	if job.Status == StatusFailed {
		response["error"] = job.Error
	}

	json.NewEncoder(w).Encode(response)
}

func findStaticDir() string {
	// List of possible static directory locations
	candidates := []string{
		"./static",                                      // Current directory (dev mode, Windows)
		"static",                                        // Relative path
		"/opt/homebrew/share/transcriber-pro/static",   // Homebrew (Apple Silicon)
		"/usr/local/share/transcriber-pro/static",      // Homebrew (Intel)
		filepath.Join(filepath.Dir(os.Args[0]), "static"), // Next to binary
	}

	for _, dir := range candidates {
		if absDir, err := filepath.Abs(dir); err == nil {
			if _, err := os.Stat(absDir); err == nil {
				return absDir
			}
		}
	}

	return ""
}

func openBrowser(url string) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}

	if err := cmd.Start(); err != nil {
		log.Printf("Failed to open browser: %v", err)
	}
}

func handleQueue(w http.ResponseWriter, r *http.Request) {
	queuedJobs, completedJobs := engine.GetQueue()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"queue":     queuedJobs,
		"completed": completedJobs,
		"count":     len(queuedJobs),
	})
}

func handleClearCompleted(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	engine.ClearCompletedJobs()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "cleared",
	})
}

func handleClearAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	engine.ClearAllJobs()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "cleared",
	})
}

func handleCancelJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract job ID from URL path: /cancel-job/{jobID}
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 {
		sendJSONError(w, "Job ID required", http.StatusBadRequest)
		return
	}
	jobID := parts[2]

	err := engine.CancelJob(jobID)
	if err != nil {
		sendJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "cancelled",
		"jobId":  jobID,
	})
}

func handleKillJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract job ID from URL path: /kill-job/{jobID}
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 {
		sendJSONError(w, "Job ID required", http.StatusBadRequest)
		return
	}
	jobID := parts[2]

	log.Printf("[Server] Force killing job %s - terminating worker process", jobID)

	// Kill the worker process (not the server!)
	if err := engine.KillJob(jobID); err != nil {
		log.Printf("[Server] Failed to kill job %s: %v", jobID, err)
		sendJSONError(w, fmt.Sprintf("Failed to kill job: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "killed",
		"jobId":  jobID,
	})

	log.Printf("[Server] Job %s killed successfully, queue will continue", jobID)
}
