# Transcriber Pro

Privacy-first audio transcription using Whisper Large-v3.

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

- 100% local processing (audio never leaves your machine)
- Whisper large-v3 (98% accuracy)
- No installation required
- No admin privileges needed
- GPU acceleration (Apple Silicon, NVIDIA CUDA)

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

## Project Structure

```
transcriber-pro/
├── server/           # Go backend
│   ├── main.go      # HTTP server & API
│   ├── transcription.go  # Whisper integration
│   ├── static/      # Web UI
│   └── Makefile     # Build automation
└── .github/         # CI/CD workflows
```

## API

### POST /transcribe

Upload audio file for transcription.

```bash
curl -X POST http://localhost:8456/transcribe \
  -F "audio=@file.mp3" \
  -F "language=en"
```

Response:

```json
{
  "job_id": "uuid",
  "status": "processing"
}
```

### GET /progress/:job_id

Poll transcription progress.

Response:

```json
{
  "status": "transcribing",
  "progress": 45.2,
  "message": "Transcribing... 45%"
}
```

## License

MIT
