package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
)

type JobStatus string

const (
	StatusQueued      JobStatus = "queued"
	StatusProcessing  JobStatus = "processing"
	StatusTranscribing JobStatus = "transcribing"
	StatusCompleted   JobStatus = "completed"
	StatusFailed      JobStatus = "failed"
)

type Job struct {
	ID           string
	Status       JobStatus
	Progress     float64
	Message      string
	ETA          string // Estimated time remaining
	Result       *TranscriptionResult
	Error        string
	FileName     string // Original filename for display
	QueuePosition int    // Position in queue (0 if not queued)
	AudioPath    string // Path to audio file
	Language     string // Language for transcription
}

type TranscriptionResult struct {
	Text     string              `json:"text"`
	Segments []TranscriptionSegment `json:"segments"`
	Language string              `json:"language"`
}

type TranscriptionSegment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

type TranscriptionEngine struct {
	model            whisper.Model
	jobs             map[string]*Job
	jobsMutex        sync.RWMutex
	modelPath        string
	queue            []string           // Queue of job IDs waiting to be processed
	queueMutex       sync.Mutex
	isProcessing     bool               // Whether a job is currently being processed
	processingCond   *sync.Cond         // Condition variable for queue processing
	cancelledJobs    map[string]bool    // Track cancelled jobs
	cancelledJobsMux sync.RWMutex       // Mutex for cancelledJobs map
	workerCmd        *exec.Cmd          // Currently running worker process
	workerMutex      sync.Mutex         // Mutex for worker command
}

func NewTranscriptionEngine() (*TranscriptionEngine, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	modelDir := filepath.Join(homeDir, ".cache", "whisper")
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create model directory: %w", err)
	}

	modelPath := filepath.Join(modelDir, "ggml-large-v3.bin")
	expectedSize := int64(3094000000) // ~3GB (approximate size of large-v3 model)

	// Check if model file exists and validate it
	needsDownload := false
	if stat, err := os.Stat(modelPath); os.IsNotExist(err) {
		needsDownload = true
	} else if stat.Size() < expectedSize-100000000 { // Allow 100MB tolerance for compression differences
		log.Printf("Model file appears incomplete (size: %d bytes, expected: ~%d bytes). Removing and re-downloading...", stat.Size(), expectedSize)
		os.Remove(modelPath)
		needsDownload = true
	}

	if needsDownload {
		log.Printf("Downloading Whisper large-v3 model (~3GB)...")
		if err := downloadModel(modelPath); err != nil {
			// Clean up partial download on failure
			os.Remove(modelPath)
			return nil, fmt.Errorf("failed to download model: %w", err)
		}
	}

	log.Printf("Loading Whisper model from %s", modelPath)
	model, err := whisper.New(modelPath)
	if err != nil {
		// If loading fails, the file might be corrupted - remove it
		log.Printf("Failed to load model (file may be corrupted), removing: %v", err)
		os.Remove(modelPath)
		return nil, fmt.Errorf("failed to load model: %w (corrupted file removed, please restart to re-download)", err)
	}

	engine := &TranscriptionEngine{
		model:         model,
		jobs:          make(map[string]*Job),
		modelPath:     modelPath,
		queue:         make([]string, 0),
		cancelledJobs: make(map[string]bool),
	}
	engine.processingCond = sync.NewCond(&engine.queueMutex)

	// Start queue processor
	go engine.processQueue()

	return engine, nil
}

func downloadModel(modelPath string) error {
	url := "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3.bin"

	cmd := exec.Command("curl", "-L", "-o", modelPath, url, "--progress-bar")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func (e *TranscriptionEngine) CreateJob(jobID, fileName, audioPath, language string) {
	e.jobsMutex.Lock()
	e.jobs[jobID] = &Job{
		ID:        jobID,
		Status:    StatusQueued,
		Progress:  0,
		Message:   "Waiting in queue...",
		FileName:  fileName,
		AudioPath: audioPath,
		Language:  language,
	}
	e.jobsMutex.Unlock()

	// Add to queue
	e.queueMutex.Lock()
	e.queue = append(e.queue, jobID)
	queuePos := len(e.queue)
	e.queueMutex.Unlock()

	// Update queue positions for all jobs
	e.updateQueuePositions()

	log.Printf("[Job %s] Added to queue at position %d", jobID, queuePos)

	// Signal queue processor
	e.processingCond.Signal()
}

func (e *TranscriptionEngine) GetJob(jobID string) *Job {
	e.jobsMutex.RLock()
	defer e.jobsMutex.RUnlock()

	if job, ok := e.jobs[jobID]; ok {
		jobCopy := *job
		return &jobCopy
	}
	return nil
}

func (e *TranscriptionEngine) GetQueue() ([]Job, []Job) {
	e.queueMutex.Lock()
	defer e.queueMutex.Unlock()

	e.jobsMutex.RLock()
	defer e.jobsMutex.RUnlock()

	// Get jobs in queue (queued + processing)
	queuedJobs := make([]Job, 0)
	for _, jobID := range e.queue {
		if job, ok := e.jobs[jobID]; ok {
			jobCopy := *job
			queuedJobs = append(queuedJobs, jobCopy)
		}
	}

	// Get completed/failed jobs (not in queue anymore)
	completedJobs := make([]Job, 0)
	completedIDs := make([]string, 0)
	for jobID, job := range e.jobs {
		if job.Status == StatusCompleted || job.Status == StatusFailed {
			// Check if it's not in the queue
			inQueue := false
			for _, queuedJobID := range e.queue {
				if queuedJobID == job.ID {
					inQueue = true
					break
				}
			}
			if !inQueue {
				completedIDs = append(completedIDs, jobID)
			}
		}
	}

	// Sort IDs to ensure consistent ordering
	sort.Strings(completedIDs)

	// Build completed jobs array in sorted order
	for _, jobID := range completedIDs {
		if job, ok := e.jobs[jobID]; ok {
			jobCopy := *job
			completedJobs = append(completedJobs, jobCopy)
		}
	}

	return queuedJobs, completedJobs
}

func (e *TranscriptionEngine) updateQueuePositions() {
	// Always acquire locks in consistent order: jobsMutex first, then queueMutex
	e.jobsMutex.Lock()
	defer e.jobsMutex.Unlock()

	e.queueMutex.Lock()
	defer e.queueMutex.Unlock()

	for i, jobID := range e.queue {
		if job, ok := e.jobs[jobID]; ok {
			job.QueuePosition = i + 1
			if i == 0 && e.isProcessing {
				job.Message = "Processing..."
			} else {
				job.Message = fmt.Sprintf("Waiting in queue (position %d)", i+1)
			}
		}
	}
}

func (e *TranscriptionEngine) processQueue() {
	for {
		e.queueMutex.Lock()

		// Wait while queue is empty
		for len(e.queue) == 0 {
			e.processingCond.Wait()
		}

		// Get next job from queue
		jobID := e.queue[0]
		e.isProcessing = true
		e.queueMutex.Unlock()

		// Get job details
		e.jobsMutex.RLock()
		job := e.jobs[jobID]
		audioPath := ""
		language := ""
		fileName := ""
		wasCancelled := false
		if job != nil {
			audioPath = job.AudioPath
			language = job.Language
			fileName = job.FileName
			wasCancelled = (job.Status == StatusFailed && job.Error == "Cancelled by user")
		}
		e.jobsMutex.RUnlock()

		if job != nil && audioPath != "" && !wasCancelled {
			log.Printf("[Queue] Processing job %s (%s)", jobID, fileName)

			// Actually call Transcribe - this blocks until complete
			e.Transcribe(context.Background(), jobID, audioPath, language, fileName)

			// Clean up audio file
			os.Remove(audioPath)
		} else if wasCancelled {
			log.Printf("[Queue] Skipping cancelled job %s (%s)", jobID, fileName)
			// Clean up audio file
			if audioPath != "" {
				os.Remove(audioPath)
			}
		}

		// Remove from queue
		e.queueMutex.Lock()
		if len(e.queue) > 0 {
			e.queue = e.queue[1:]
		}
		e.isProcessing = false
		e.queueMutex.Unlock()

		e.updateQueuePositions()
		log.Printf("[Queue] Job %s completed, %d jobs remaining", jobID, len(e.queue))
	}
}

func (e *TranscriptionEngine) Transcribe(ctx context.Context, jobID, audioPath, language, originalFileName string) {
	duration, err := getAudioDuration(audioPath)
	if err != nil {
		e.updateJob(jobID, StatusFailed, 0, "", "", nil, fmt.Sprintf("Failed to get audio duration: %v", err))
		return
	}

	var speedFactor float64
	if runtime.GOARCH == "arm64" && runtime.GOOS == "darwin" {
		speedFactor = 6.0
	} else {
		speedFactor = 1.5
	}

	expectedTime := duration / speedFactor
	startTime := time.Now()

	stopEstimator := make(chan struct{})
	go e.estimateProgress(jobID, startTime, expectedTime, stopEstimator)

	// Prepare worker request
	type WorkerRequest struct {
		JobID     string `json:"jobID"`
		AudioPath string `json:"audioPath"`
		ModelPath string `json:"modelPath"`
		Language  string `json:"language"`
	}

	req := WorkerRequest{
		JobID:     jobID,
		AudioPath: audioPath,
		ModelPath: e.modelPath,
		Language:  language,
	}

	reqJSON, err := json.Marshal(req)
	if err != nil {
		close(stopEstimator)
		e.updateJob(jobID, StatusFailed, 0, "", "", nil, fmt.Sprintf("Failed to create worker request: %v", err))
		return
	}

	// Get the worker binary path - use absolute path of current executable
	exePath, err := os.Executable()
	if err != nil {
		close(stopEstimator)
		e.updateJob(jobID, StatusFailed, 0, "", "", nil, fmt.Sprintf("Failed to get executable path: %v", err))
		return
	}
	workerPath := filepath.Join(filepath.Dir(exePath), "transcriber-worker")
	log.Printf("[Job %s] Starting worker: %s", jobID, workerPath)

	// Start worker process
	cmd := exec.Command(workerPath, string(reqJSON))
	cmd.Stderr = os.Stderr

	// Store the command so we can kill it later
	e.workerMutex.Lock()
	e.workerCmd = cmd
	e.workerMutex.Unlock()

	// Run worker and capture output
	output, err := cmd.Output()

	// Clear the worker command
	e.workerMutex.Lock()
	e.workerCmd = nil
	e.workerMutex.Unlock()

	close(stopEstimator)

	log.Printf("[Job %s] Worker finished, output length: %d bytes", jobID, len(output))
	if len(output) > 0 && len(output) < 1000 {
		log.Printf("[Job %s] Worker output: %s", jobID, string(output))
	}

	// Check if job was killed/cancelled
	if e.IsCancelled(jobID) {
		log.Printf("[Job %s] Job was cancelled", jobID)
		e.updateJob(jobID, StatusFailed, 0, "", "", nil, "Cancelled by user")
		return
	}

	if err != nil {
		log.Printf("[Job %s] Worker error: %v", jobID, err)
		e.updateJob(jobID, StatusFailed, 0, "", "", nil, fmt.Sprintf("Worker failed: %v", err))
		return
	}

	// Parse worker response
	type WorkerResponse struct {
		Success  bool                     `json:"success"`
		Text     string                   `json:"text,omitempty"`
		Segments []TranscriptionSegment   `json:"segments,omitempty"`
		Error    string                   `json:"error,omitempty"`
		Duration float64                  `json:"duration"`
	}

	var resp WorkerResponse
	if err := json.Unmarshal(output, &resp); err != nil {
		e.updateJob(jobID, StatusFailed, 0, "", "", nil, fmt.Sprintf("Failed to parse worker response: %v", err))
		return
	}

	if !resp.Success {
		e.updateJob(jobID, StatusFailed, 0, "", "", nil, resp.Error)
		return
	}

	result := &TranscriptionResult{
		Text:     resp.Text,
		Segments: resp.Segments,
		Language: language,
	}

	e.updateJob(jobID, StatusCompleted, 100, "Completed", "", result, "")

	// Save transcription to disk
	if err := saveTranscription(result, originalFileName); err != nil {
		log.Printf("[Job %s] Warning: Failed to save transcription to disk: %v", jobID, err)
	}
}

func (e *TranscriptionEngine) estimateProgress(jobID string, startTime time.Time, expectedTime float64, stop chan struct{}) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			// Check if job was cancelled
			if e.IsCancelled(jobID) {
				log.Printf("[Job %s] Detected cancellation during progress estimation", jobID)
				return
			}

			elapsed := time.Since(startTime).Seconds()
			progress := (elapsed / expectedTime) * 100
			if progress > 99 {
				progress = 99
			}

			// Calculate ETA
			remaining := expectedTime - elapsed
			eta := formatDuration(remaining)

			e.updateJob(jobID, StatusTranscribing, progress, fmt.Sprintf("Transcribing... %.0f%%", progress), eta, nil, "")
		}
	}
}

func formatDuration(seconds float64) string {
	if seconds < 0 {
		return "Almost done..."
	}

	duration := time.Duration(seconds) * time.Second
	hours := int(duration.Hours())
	minutes := int(duration.Minutes()) % 60
	secs := int(duration.Seconds()) % 60

	if hours > 0 {
		return fmt.Sprintf("%dh %dm remaining", hours, minutes)
	} else if minutes > 0 {
		return fmt.Sprintf("%dm %ds remaining", minutes, secs)
	} else {
		return fmt.Sprintf("%ds remaining", secs)
	}
}

func (e *TranscriptionEngine) updateJob(jobID string, status JobStatus, progress float64, message string, eta string, result *TranscriptionResult, errorMsg string) {
	e.jobsMutex.Lock()
	defer e.jobsMutex.Unlock()

	if job, ok := e.jobs[jobID]; ok {
		job.Status = status
		job.Progress = progress
		job.Message = message
		job.ETA = eta
		if result != nil {
			job.Result = result
		}
		if errorMsg != "" {
			job.Error = errorMsg
		}
	}
}

func getAudioDuration(audioPath string) (float64, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		audioPath)

	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	durationStr := strings.TrimSpace(string(output))
	duration, err := strconv.ParseFloat(durationStr, 64)
	if err != nil {
		return 0, err
	}

	return duration, nil
}

func (e *TranscriptionEngine) Close() {
	if e.model != nil {
		e.model.Close()
	}
}

// getOutputDir returns the platform-specific directory for saving transcriptions
func getOutputDir() (string, error) {
	var baseDir string

	if runtime.GOOS == "darwin" {
		// macOS: Use ~/.transcriber-pro
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		baseDir = filepath.Join(homeDir, ".transcriber-pro")
	} else if runtime.GOOS == "windows" {
		// Windows: Use executable directory/transcriptions
		exePath, err := os.Executable()
		if err != nil {
			return "", err
		}
		baseDir = filepath.Join(filepath.Dir(exePath), "transcriptions")
	} else {
		// Linux/Other: Use ~/transcriber-pro
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		baseDir = filepath.Join(homeDir, "transcriber-pro")
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return "", err
	}

	return baseDir, nil
}

// saveTranscription saves the transcription result to disk in multiple formats
func saveTranscription(result *TranscriptionResult, originalFileName string) error {
	outputDir, err := getOutputDir()
	if err != nil {
		return fmt.Errorf("failed to get output directory: %w", err)
	}

	// Create timestamp prefix: YYYYMMDD_HHMMSS
	timestamp := time.Now().Format("20060102_150405")

	// Remove extension from original filename
	baseFilename := strings.TrimSuffix(originalFileName, filepath.Ext(originalFileName))

	// Create folder name: timestamp_filename
	folderName := fmt.Sprintf("%s_%s", timestamp, baseFilename)
	outputFolder := filepath.Join(outputDir, folderName)

	// Create the output folder
	if err := os.MkdirAll(outputFolder, 0755); err != nil {
		return fmt.Errorf("failed to create output folder: %w", err)
	}

	// Save as TXT
	txtPath := filepath.Join(outputFolder, "transcript.txt")
	if err := os.WriteFile(txtPath, []byte(result.Text), 0644); err != nil {
		return fmt.Errorf("failed to save TXT: %w", err)
	}

	// Save as JSON
	jsonPath := filepath.Join(outputFolder, "transcript.json")
	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	if err := os.WriteFile(jsonPath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to save JSON: %w", err)
	}

	// Save as SRT
	srtPath := filepath.Join(outputFolder, "transcript.srt")
	srtContent := generateSRT(result.Segments)
	if err := os.WriteFile(srtPath, []byte(srtContent), 0644); err != nil {
		return fmt.Errorf("failed to save SRT: %w", err)
	}

	log.Printf("Transcription saved to: %s", outputFolder)
	return nil
}

// generateSRT creates SRT subtitle format from segments
func generateSRT(segments []TranscriptionSegment) string {
	var srt strings.Builder

	for i, segment := range segments {
		// Segment number
		srt.WriteString(fmt.Sprintf("%d\n", i+1))

		// Timestamps
		startTime := formatSRTTime(segment.Start)
		endTime := formatSRTTime(segment.End)
		srt.WriteString(fmt.Sprintf("%s --> %s\n", startTime, endTime))

		// Text
		srt.WriteString(segment.Text)
		srt.WriteString("\n\n")
	}

	return srt.String()
}

// formatSRTTime formats seconds to SRT timestamp format (HH:MM:SS,mmm)
func formatSRTTime(seconds float64) string {
	hours := int(seconds / 3600)
	minutes := int((seconds - float64(hours*3600)) / 60)
	secs := int(seconds - float64(hours*3600) - float64(minutes*60))
	millis := int((seconds - float64(int(seconds))) * 1000)

	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, secs, millis)
}

func (e *TranscriptionEngine) ClearCompletedJobs() {
	e.jobsMutex.Lock()
	defer e.jobsMutex.Unlock()

	e.queueMutex.Lock()
	defer e.queueMutex.Unlock()

	// Remove all completed/failed jobs that are not in queue
	for jobID, job := range e.jobs {
		if job.Status == StatusCompleted || job.Status == StatusFailed {
			// Check if it's not in the queue
			inQueue := false
			for _, queuedJobID := range e.queue {
				if queuedJobID == jobID {
					inQueue = true
					break
				}
			}
			if !inQueue {
				delete(e.jobs, jobID)
			}
		}
	}

	log.Printf("[Queue] Cleared completed jobs")
}

// ClearAllJobs clears all jobs (both queued and completed), except the currently processing one
func (e *TranscriptionEngine) ClearAllJobs() {
	e.jobsMutex.Lock()
	e.queueMutex.Lock()

	// Keep only the first job in queue if it's processing
	var currentJobID string
	if len(e.queue) > 0 && e.isProcessing {
		currentJobID = e.queue[0]
		// Clear the queue except for the first (processing) job
		e.queue = e.queue[:1]
	} else {
		// No job is processing, clear entire queue
		e.queue = nil
	}

	// Delete all jobs except the one currently processing
	for jobID := range e.jobs {
		if jobID != currentJobID {
			delete(e.jobs, jobID)
		}
	}

	e.queueMutex.Unlock()
	e.jobsMutex.Unlock()

	// Update positions after releasing locks to avoid deadlock
	e.updateQueuePositions()
	log.Printf("[Queue] Cleared all jobs")
}

// CancelJob removes a job from the queue or aborts an active transcription
func (e *TranscriptionEngine) CancelJob(jobID string) error {
	// Mark job as cancelled first (no other locks needed)
	e.cancelledJobsMux.Lock()
	e.cancelledJobs[jobID] = true
	e.cancelledJobsMux.Unlock()

	// Find and remove from queue
	e.queueMutex.Lock()
	found := false
	isFirstJob := false
	isProcessingJob := false
	newQueue := make([]string, 0)
	for i, queuedJobID := range e.queue {
		if queuedJobID == jobID {
			found = true
			isFirstJob = (i == 0)
			isProcessingJob = isFirstJob && e.isProcessing
			log.Printf("[Queue] Cancelling job %s (isProcessing: %v, isFirstJob: %v)", jobID, e.isProcessing, isFirstJob)
			// Don't add to new queue
			continue
		}
		newQueue = append(newQueue, queuedJobID)
	}

	if !found {
		e.queueMutex.Unlock()
		return fmt.Errorf("job not found in queue")
	}

	e.queue = newQueue
	e.queueMutex.Unlock()

	// Now update job status (separate lock, after releasing queueMutex)
	e.jobsMutex.Lock()
	if job, ok := e.jobs[jobID]; ok {
		job.Status = StatusFailed
		job.Error = "Cancelled by user"
		if isProcessingJob {
			job.Message = "Cancelling..."
		} else {
			job.Message = "Cancelled"
		}
	}
	e.jobsMutex.Unlock()

	// Update queue positions
	e.updateQueuePositions()
	return nil
}

// IsCancelled checks if a job has been cancelled
func (e *TranscriptionEngine) IsCancelled(jobID string) bool {
	e.cancelledJobsMux.RLock()
	defer e.cancelledJobsMux.RUnlock()
	return e.cancelledJobs[jobID]
}

// KillJob kills the currently running worker process
func (e *TranscriptionEngine) KillJob(jobID string) error {
	// Mark job as cancelled
	e.cancelledJobsMux.Lock()
	e.cancelledJobs[jobID] = true
	e.cancelledJobsMux.Unlock()

	// Kill the worker process if it's running
	e.workerMutex.Lock()
	cmd := e.workerCmd
	e.workerMutex.Unlock()

	if cmd != nil && cmd.Process != nil {
		log.Printf("[Job %s] Killing worker process (PID: %d)", jobID, cmd.Process.Pid)
		if err := cmd.Process.Kill(); err != nil {
			return fmt.Errorf("failed to kill worker process: %w", err)
		}
		log.Printf("[Job %s] Worker process killed successfully", jobID)
	}

	// Mark job as failed
	e.jobsMutex.Lock()
	if job, ok := e.jobs[jobID]; ok {
		job.Status = StatusFailed
		job.Error = "Killed by user"
		job.Message = "Killed"
	}
	e.jobsMutex.Unlock()

	return nil
}
