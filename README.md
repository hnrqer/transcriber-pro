# Transcriber Pro

Privacy-first audio transcription using Whisper Large-v3. All processing happens locally on your machine with a modern web UI.

## Quick Start

### macOS

```bash
brew tap hnrqer/transcriber-pro https://github.com/hnrqer/transcriber-pro
brew install transcriber-pro
transcriber-pro
```

### Windows

1. Download and extract transcriber-pro-windows.zip
2. Double-click start.bat
3. Browser opens automatically

## Features

- **100% Local Processing** - Audio never leaves your machine
- **High Accuracy** - Whisper large-v3 model (98% accuracy)
- **Queue Management** - Upload multiple files, process serially
- **Real-time Progress** - Live updates via WebSocket
- **Killable Jobs** - Cancel or kill running transcriptions instantly
- **Multiple Export Formats** - TXT, SRT, JSON
- **GPU Acceleration** - Optimized for Apple Silicon, NVIDIA CUDA
- **Modern Web UI** - Drag-and-drop, two-column layout with scrollable queue
- **No Installation Required** - Single binary, no dependencies
- **No Admin Privileges Needed** - Runs in user space

## System Requirements

- macOS 11+ or Windows 10+
- 8GB RAM (16GB recommended)
- 5GB disk space for Whisper model
- GPU recommended for 10x faster transcription

## First Run

On first run, the Whisper large-v3 model (~3GB) downloads automatically to `~/.cache/whisper`.
This may take 5-10 minutes depending on your internet connection.

## Building from Source

### Requirements

- Go 1.25+
- CMake
- FFmpeg
- whisper.cpp v1.8.2 (automatically cloned)

### macOS

```bash
brew install go cmake ffmpeg
cd server
make
export DYLD_LIBRARY_PATH="../whisper.cpp/build/src:../whisper.cpp/build/ggml/src:$DYLD_LIBRARY_PATH"
./transcriber-pro
```

### Windows

```cmd
choco install golang cmake ffmpeg
cd server
go build -o transcriber-pro.exe
transcriber-pro.exe
```

## Architecture

Transcriber Pro uses a **worker process architecture** for robust job control:

- **Main Server** (`transcriber-pro`) - HTTP server, WebSocket handler, queue management
- **Worker Process** (`transcriber-worker`) - Spawned per job, handles actual transcription
- **Benefits**: Kill transcription jobs without affecting server, better resource isolation

## Project Structure

```
transcriber-pro/
├── server/           # Go backend
│   ├── main.go      # HTTP server, WebSocket handler, API endpoints
│   ├── transcription.go  # Queue management, job orchestration
│   ├── worker/      # Worker process for transcription
│   │   └── main.go  # Whisper.cpp integration
│   ├── static/      # Web UI
│   │   ├── index.html   # HTML structure
│   │   ├── app.js       # WebSocket client, queue rendering
│   │   └── style.css    # Two-column layout, styling
│   └── Makefile     # Build automation (whisper.cpp + Go binaries)
└── .github/         # CI/CD workflows
```

## API

### POST /transcribe

Upload audio file(s) for transcription. Supports multiple files in a single request.

```bash
curl -X POST http://localhost:8456/transcribe \
  -F "audio=@file1.mp3" \
  -F "audio=@file2.mp3" \
  -F "language=en"
```

Response:

```json
{
  "job_ids": ["uuid1", "uuid2"],
  "message": "2 jobs queued"
}
```

### GET /queue

Get current queue state including active, queued, completed, and failed jobs.

Response:

```json
{
  "queue": [
    {
      "ID": "uuid",
      "FileName": "audio.mp3",
      "Status": "processing",
      "Progress": 45.2,
      "QueuePosition": 0
    }
  ],
  "completed": [
    {
      "ID": "uuid2",
      "FileName": "done.mp3",
      "Status": "completed",
      "Text": "Transcription text...",
      "Language": "en",
      "Duration": 120.5
    }
  ]
}
```

### POST /kill-job/:job_id

Kill a running transcription job by terminating the worker process.

Response:

```json
{
  "message": "Job killed successfully"
}
```

### POST /cancel-job/:job_id

Cancel a queued job (not yet processing).

Response:

```json
{
  "message": "Job cancelled successfully"
}
```

### POST /clear-completed

Remove all completed and failed jobs from the queue.

Response:

```json
{
  "message": "Completed jobs cleared"
}
```

### POST /clear-all

Clear all jobs (queued, completed, and failed). Does not affect currently processing jobs.

Response:

```json
{
  "message": "All jobs cleared"
}
```

### GET /queue (Polling)

The UI polls this endpoint every 500ms for real-time updates on queue changes, job progress, and completion events.

## Testing

End-to-end tests using Playwright:

```bash
cd tests
npm install
npx playwright install
npm test
```

Tests cover:
- Homepage loading and UI elements
- Connection status
- Queue management
- API endpoints (/health, /version, /queue, /clear-all, etc.)
- File upload validation
- Drag and drop interactions

See [tests/README.md](tests/README.md) for more details.

## License

MIT
