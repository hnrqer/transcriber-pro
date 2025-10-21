package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
)

// WorkerRequest is the input data for the worker
type WorkerRequest struct {
	JobID      string `json:"jobID"`
	AudioPath  string `json:"audioPath"`
	ModelPath  string `json:"modelPath"`
	Language   string `json:"language"`
}

// WorkerResponse is the output data from the worker
type WorkerResponse struct {
	Success   bool                   `json:"success"`
	Text      string                 `json:"text,omitempty"`
	Segments  []TranscriptionSegment `json:"segments,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Duration  float64                `json:"duration"`
}

// TranscriptionSegment represents a single segment of transcribed text
type TranscriptionSegment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: worker <request-json>")
	}

	// Parse request
	var req WorkerRequest
	if err := json.Unmarshal([]byte(os.Args[1]), &req); err != nil {
		sendError(fmt.Sprintf("Failed to parse request: %v", err))
		return
	}

	log.Printf("[Worker %s] Starting transcription for %s", req.JobID, req.AudioPath)
	startTime := time.Now()

	// Load model
	model, err := whisper.New(req.ModelPath)
	if err != nil {
		sendError(fmt.Sprintf("Failed to load model: %v", err))
		return
	}
	defer model.Close()

	// Load audio
	audioData, err := loadAudioData(req.AudioPath)
	if err != nil {
		sendError(fmt.Sprintf("Failed to load audio: %v", err))
		return
	}

	// Create context
	context, err := model.NewContext()
	if err != nil {
		sendError(fmt.Sprintf("Failed to create context: %v", err))
		return
	}

	// Set language if specified
	if req.Language != "" && req.Language != "auto" {
		context.SetLanguage(req.Language)
	}

	// Process audio
	log.Printf("[Worker %s] Processing audio...", req.JobID)
	if err := context.Process(audioData, nil, nil, nil); err != nil {
		sendError(fmt.Sprintf("Failed to process audio: %v", err))
		return
	}

	// Extract transcription
	var fullText string
	var segments []TranscriptionSegment

	for {
		segment, err := context.NextSegment()
		if err != nil {
			break
		}

		text := segment.Text
		fullText += text + " "

		segments = append(segments, TranscriptionSegment{
			Start: float64(segment.Start.Milliseconds()) / 1000.0,
			End:   float64(segment.End.Milliseconds()) / 1000.0,
			Text:  text,
		})
	}

	duration := time.Since(startTime).Seconds()
	log.Printf("[Worker %s] Transcription complete in %.2fs", req.JobID, duration)

	// Send success response
	resp := WorkerResponse{
		Success:  true,
		Text:     fullText,
		Segments: segments,
		Duration: duration,
	}

	data, _ := json.Marshal(resp)
	fmt.Println(string(data))
}

func sendError(errMsg string) {
	log.Printf("[Worker] Error: %s", errMsg)
	resp := WorkerResponse{
		Success: false,
		Error:   errMsg,
	}
	data, _ := json.Marshal(resp)
	fmt.Println(string(data))
	os.Exit(1)
}

func loadAudioData(audioPath string) ([]float32, error) {
	wavPath := audioPath + ".wav"
	defer os.Remove(wavPath)

	cmd := exec.Command("ffmpeg",
		"-i", audioPath,
		"-ar", "16000",
		"-ac", "1",
		"-c:a", "pcm_s16le",
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

	// Read as 16-bit signed integers
	int16Samples := make([]int16, dataSize/2)
	if err := binary.Read(file, binary.LittleEndian, &int16Samples); err != nil {
		return nil, err
	}

	// Convert int16 to float32 (normalized to -1.0 to 1.0)
	samples := make([]float32, len(int16Samples))
	for i, sample := range int16Samples {
		samples[i] = float32(sample) / 32768.0
	}

	return samples, nil
}

func shellQuote(s string) string {
	return "\"" + s + "\""
}
