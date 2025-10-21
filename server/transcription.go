package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
)

type JobStatus string

const (
	StatusProcessing  JobStatus = "processing"
	StatusTranscribing JobStatus = "transcribing"
	StatusCompleted   JobStatus = "completed"
	StatusFailed      JobStatus = "failed"
)

type Job struct {
	ID       string
	Status   JobStatus
	Progress float64
	Message  string
	ETA      string // Estimated time remaining
	Result   *TranscriptionResult
	Error    string
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
	model     whisper.Model
	jobs      map[string]*Job
	jobsMutex sync.RWMutex
	modelPath string
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

	return &TranscriptionEngine{
		model:     model,
		jobs:      make(map[string]*Job),
		modelPath: modelPath,
	}, nil
}

func downloadModel(modelPath string) error {
	url := "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3.bin"

	cmd := exec.Command("curl", "-L", "-o", modelPath, url, "--progress-bar")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func (e *TranscriptionEngine) CreateJob(jobID string) {
	e.jobsMutex.Lock()
	defer e.jobsMutex.Unlock()

	e.jobs[jobID] = &Job{
		ID:       jobID,
		Status:   StatusProcessing,
		Progress: 0,
		Message:  "Starting transcription...",
	}
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

func (e *TranscriptionEngine) Transcribe(ctx context.Context, jobID, audioPath, language string) {
	duration, err := getAudioDuration(audioPath)
	if err != nil {
		e.updateJob(jobID, StatusFailed, 0, "", "", nil, fmt.Sprintf("Failed to get audio duration: %v", err))
		return
	}

	audioData, err := loadAudioFile(audioPath)
	if err != nil {
		e.updateJob(jobID, StatusFailed, 0, "", "", nil, fmt.Sprintf("Failed to load audio: %v", err))
		return
	}

	context, err := e.model.NewContext()
	if err != nil {
		e.updateJob(jobID, StatusFailed, 0, "", "", nil, fmt.Sprintf("Failed to create context: %v", err))
		return
	}

	if err := context.SetLanguage(language); err != nil {
		e.updateJob(jobID, StatusFailed, 0, "", "", nil, fmt.Sprintf("Failed to set language: %v", err))
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

	err = context.Process(audioData, nil, nil, nil)

	close(stopEstimator)

	if err != nil {
		e.updateJob(jobID, StatusFailed, 0, "", "", nil, fmt.Sprintf("Transcription failed: %v", err))
		return
	}

	result := &TranscriptionResult{
		Text:     "",
		Segments: []TranscriptionSegment{},
		Language: language,
	}

	for {
		segment, err := context.NextSegment()
		if err != nil {
			break
		}

		result.Segments = append(result.Segments, TranscriptionSegment{
			Start: float64(segment.Start.Milliseconds()) / 1000.0,
			End:   float64(segment.End.Milliseconds()) / 1000.0,
			Text:  segment.Text,
		})
		result.Text += segment.Text
	}

	e.updateJob(jobID, StatusCompleted, 100, "Completed", "", result, "")
}

func (e *TranscriptionEngine) estimateProgress(jobID string, startTime time.Time, expectedTime float64, stop chan struct{}) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
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

func loadAudioFile(audioPath string) ([]float32, error) {
	wavPath := audioPath + ".wav"
	defer os.Remove(wavPath)

	cmd := exec.Command("ffmpeg",
		"-i", audioPath,
		"-ar", "16000",
		"-ac", "1",
		"-c:a", "pcm_f32le",
		"-f", "wav",
		"-y",
		wavPath)

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg conversion failed: %w", err)
	}

	file, err := os.Open(wavPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}

	headerSize := int64(44)
	dataSize := stat.Size() - headerSize

	if _, err := file.Seek(headerSize, 0); err != nil {
		return nil, err
	}

	samples := make([]float32, dataSize/4)
	if err := binary.Read(file, binary.LittleEndian, &samples); err != nil {
		return nil, err
	}

	return samples, nil
}

func (e *TranscriptionEngine) Close() {
	if e.model != nil {
		e.model.Close()
	}
}
