/**
 * Transcriber Pro - PWA Application
 * Handles server detection, file upload, and transcription
 */

class WhisperApp {
    constructor() {
        // UI is served by server at same origin
        this.serverUrl = window.location.origin;
        this.currentFile = null;
        this.transcriptionResult = null;
        this.uploadQueue = []; // Local queue of files to upload
        this.activeJobs = new Map(); // Map of job_id -> file info
        this.completedJobs = []; // Array of completed transcriptions

        // UI elements
        this.elements = {
            connectionStatus: document.getElementById('connectionStatus'),
            statusText: document.querySelector('.status-text'),
            serverInstructions: document.getElementById('serverInstructions'),
            uploadSection: document.getElementById('uploadSection'),
            processingSection: document.getElementById('processingSection'),
            resultsSection: document.getElementById('resultsSection'),

            dropZone: document.getElementById('dropZone'),
            fileInput: document.getElementById('fileInput'),
            languageSelect: document.getElementById('languageSelect'),

            processingFileName: document.getElementById('processingFileName'),
            processingStatus: document.getElementById('processingStatus'),
            progressFill: document.getElementById('progressFill'),
            progressPercent: document.getElementById('progressPercent'),
            etaText: document.getElementById('etaText'),
            etaValue: document.getElementById('etaValue'),

            detectedLanguage: document.getElementById('detectedLanguage'),
            audioDuration: document.getElementById('audioDuration'),
            wordCount: document.getElementById('wordCount'),
            transcriptText: document.getElementById('transcriptText'),
            segmentsList: document.getElementById('segmentsList'),

            retryConnection: document.getElementById('retryConnection'),
            newTranscription: document.getElementById('newTranscription'),
            copyBtn: document.getElementById('copyBtn'),
            exportTxt: document.getElementById('exportTxt'),
            exportSrt: document.getElementById('exportSrt'),
            exportJson: document.getElementById('exportJson')
        };

        this.init();
    }

    async init() {
        console.log('[WhisperApp] Initializing...');
        this.setupEventListeners();

        // Verify server server is responding
        await this.detectCompanion();

        // Fetch and display version
        await this.fetchVersion();
    }

    setupEventListeners() {
        // Drop zone events
        this.elements.dropZone.addEventListener('click', () => {
            this.elements.fileInput.click();
        });

        this.elements.dropZone.addEventListener('dragover', (e) => {
            e.preventDefault();
            this.elements.dropZone.classList.add('drag-over');
        });

        this.elements.dropZone.addEventListener('dragleave', () => {
            this.elements.dropZone.classList.remove('drag-over');
        });

        this.elements.dropZone.addEventListener('drop', (e) => {
            e.preventDefault();
            this.elements.dropZone.classList.remove('drag-over');

            const files = e.dataTransfer.files;
            if (files.length > 0) {
                this.handleFileSelection(files[0]);
            }
        });

        this.elements.fileInput.addEventListener('change', (e) => {
            if (e.target.files.length > 0) {
                this.handleFileSelection(e.target.files[0]);
            }
        });

        // Button events
        this.elements.retryConnection.addEventListener('click', () => {
            window.location.reload();
        });

        this.elements.newTranscription.addEventListener('click', () => {
            this.resetToUpload();
        });

        this.elements.copyBtn.addEventListener('click', () => {
            this.copyTranscript();
        });

        this.elements.exportTxt.addEventListener('click', () => {
            this.exportAs('txt');
        });

        this.elements.exportSrt.addEventListener('click', () => {
            this.exportAs('srt');
        });

        this.elements.exportJson.addEventListener('click', () => {
            this.exportAs('json');
        });
    }

    async detectCompanion() {
        console.log('[WhisperApp] Checking server connection...');

        try {
            const response = await fetch('/health', {
                method: 'GET',
                signal: AbortSignal.timeout(2000)
            });

            if (response.ok) {
                const data = await response.json();
                this.onCompanionConnected(data);
            } else {
                this.onCompanionDisconnected();
            }
        } catch (error) {
            console.error('[WhisperApp] Connection check failed:', error);
            this.onCompanionDisconnected();
        }
    }

    async fetchVersion() {
        try {
            const response = await fetch('/version');
            if (response.ok) {
                const data = await response.json();
                const versionElement = document.getElementById('version');
                if (versionElement && data.version) {
                    versionElement.textContent = `v${data.version}`;
                }
            }
        } catch (error) {
            console.error('[WhisperApp] Failed to fetch version:', error);
        }
    }

    onCompanionConnected(info) {
        console.log('[WhisperApp] Companion connected:', info);

        this.elements.connectionStatus.className = 'connection-status connected';
        this.elements.statusText.textContent = 'Connected';

        this.elements.serverInstructions.style.display = 'none';
        this.elements.uploadSection.style.display = 'block';
    }

    onCompanionDisconnected() {
        console.log('[WhisperApp] Companion not found');

        this.serverUrl = null;
        this.elements.connectionStatus.className = 'connection-status disconnected';
        this.elements.statusText.textContent = 'Companion not running';

        this.elements.serverInstructions.style.display = 'block';
        this.elements.uploadSection.style.display = 'none';
        this.elements.processingSection.style.display = 'none';
        this.elements.resultsSection.style.display = 'none';
    }

    async handleFileSelection(file) {
        console.log('[WhisperApp] File selected:', file.name);

        if (!this.serverUrl) {
            alert('Companion not connected. Please start the server app first.');
            return;
        }

        this.currentFile = file;
        this.showProcessingView();
        await this.transcribeFile(file);
    }

    showProcessingView() {
        this.elements.uploadSection.style.display = 'none';
        this.elements.resultsSection.style.display = 'none';
        this.elements.processingSection.style.display = 'block';

        this.elements.processingFileName.textContent = this.currentFile.name;
        this.elements.processingStatus.textContent = 'Uploading...';
        this.updateProgress(0);
    }

    updateProgress(percent) {
        this.elements.progressFill.style.width = `${percent}%`;
        this.elements.progressPercent.textContent = Math.round(percent);
    }

    async transcribeFile(file) {
        try {
            const formData = new FormData();
            formData.append('audio', file);

            const language = this.elements.languageSelect.value;
            if (language) {
                formData.append('language', language);
            }

            this.elements.processingStatus.textContent = 'Starting transcription...';
            this.updateProgress(0);

            // Step 1: Start transcription job
            const response = await fetch(`${this.serverUrl}/transcribe`, {
                method: 'POST',
                body: formData
            });

            if (!response.ok) {
                const error = await response.json();
                throw new Error(error.error || 'Failed to start transcription');
            }

            const { job_id } = await response.json();
            console.log('[WhisperApp] Job started:', job_id);

            // Step 2: Poll for REAL progress updates
            await this.pollProgress(job_id);

        } catch (error) {
            console.error('[WhisperApp] Transcription error:', error);
            alert(`Transcription failed: ${error.message}`);
            this.resetToUpload();
        }
    }

    async pollProgress(jobId) {
        /**
         * Poll the server for REAL progress updates
         * This gets actual progress from Whisper, not fake simulation
         */
        const pollInterval = 500; // Check every 500ms

        while (true) {
            try {
                const response = await fetch(`${this.serverUrl}/progress/${jobId}`);

                if (!response.ok) {
                    throw new Error('Failed to get progress');
                }

                const progress = await response.json();

                // Update UI with REAL progress
                this.updateProgress(progress.progress);
                this.elements.processingStatus.textContent = progress.message;

                // Update ETA if available
                if (progress.eta) {
                    this.elements.etaValue.textContent = progress.eta;
                    this.elements.etaText.style.display = 'block';
                } else {
                    this.elements.etaText.style.display = 'none';
                }

                console.log(`[WhisperApp] Progress: ${progress.progress.toFixed(1)}% - ${progress.message}`);

                // Check if completed
                if (progress.status === 'completed') {
                    if (progress.result) {
                        this.transcriptionResult = progress.result;
                        this.showResults(progress.result);
                    } else {
                        throw new Error('Transcription completed but no result');
                    }
                    break;
                }

                // Check if failed
                if (progress.status === 'failed') {
                    throw new Error(progress.error || 'Transcription failed');
                }

                // Wait before next poll
                await new Promise(resolve => setTimeout(resolve, pollInterval));

            } catch (error) {
                console.error('[WhisperApp] Progress polling error:', error);
                throw error;
            }
        }
    }

    showResults(result) {
        console.log('[WhisperApp] Showing results:', result);

        this.elements.processingSection.style.display = 'none';
        this.elements.resultsSection.style.display = 'block';

        // Set info
        this.elements.detectedLanguage.textContent = result.language || 'Unknown';
        this.elements.transcriptText.textContent = result.text;

        // Calculate stats
        const wordCount = result.text.trim().split(/\s+/).length;
        this.elements.wordCount.textContent = wordCount;

        if (result.segments && result.segments.length > 0) {
            const duration = result.segments[result.segments.length - 1].end;
            this.elements.audioDuration.textContent = this.formatDuration(duration);

            this.renderSegments(result.segments);
        } else {
            this.elements.audioDuration.textContent = 'Unknown';
        }
    }

    renderSegments(segments) {
        this.elements.segmentsList.innerHTML = '';

        segments.forEach((segment, index) => {
            const segmentEl = document.createElement('div');
            segmentEl.className = 'segment-item';

            const timeEl = document.createElement('div');
            timeEl.className = 'segment-time';
            timeEl.textContent = `${this.formatTime(segment.start)} → ${this.formatTime(segment.end)}`;

            const textEl = document.createElement('div');
            textEl.className = 'segment-text';
            textEl.textContent = segment.text.trim();

            segmentEl.appendChild(timeEl);
            segmentEl.appendChild(textEl);
            this.elements.segmentsList.appendChild(segmentEl);
        });
    }

    formatDuration(seconds) {
        const mins = Math.floor(seconds / 60);
        const secs = Math.floor(seconds % 60);
        return `${mins}:${secs.toString().padStart(2, '0')}`;
    }

    formatTime(seconds) {
        const mins = Math.floor(seconds / 60);
        const secs = Math.floor(seconds % 60);
        const ms = Math.floor((seconds % 1) * 1000);
        return `${mins}:${secs.toString().padStart(2, '0')}.${ms.toString().padStart(3, '0').slice(0, 2)}`;
    }

    resetToUpload() {
        this.currentFile = null;
        this.transcriptionResult = null;

        this.elements.processingSection.style.display = 'none';
        this.elements.resultsSection.style.display = 'none';
        this.elements.uploadSection.style.display = 'block';

        this.elements.fileInput.value = '';
    }

    copyTranscript() {
        if (!this.transcriptionResult) return;

        navigator.clipboard.writeText(this.transcriptionResult.text)
            .then(() => {
                this.elements.copyBtn.textContent = '✅ Copied!';
                setTimeout(() => {
                    this.elements.copyBtn.textContent = '📋 Copy';
                }, 2000);
            })
            .catch(err => {
                console.error('Copy failed:', err);
                alert('Failed to copy to clipboard');
            });
    }

    async exportAs(format) {
        if (!this.transcriptionResult) return;

        try {
            let content = '';
            let mimeType = 'text/plain';
            const filename = `transcript-${Date.now()}.${format}`;

            if (format === 'txt') {
                content = this.transcriptionResult.text;
                mimeType = 'text/plain';
            } else if (format === 'srt') {
                content = this.generateSRT();
                mimeType = 'text/plain';
            } else if (format === 'json') {
                content = JSON.stringify(this.transcriptionResult, null, 2);
                mimeType = 'application/json';
            }

            // Create and download file
            const blob = new Blob([content], { type: mimeType });
            const url = URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = filename;
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            URL.revokeObjectURL(url);

        } catch (error) {
            console.error('Export failed:', error);
            alert(`Export failed: ${error.message}`);
        }
    }

    generateSRT() {
        let srt = '';
        this.transcriptionResult.segments.forEach((segment, index) => {
            const start = this.formatSRTTime(segment.start);
            const end = this.formatSRTTime(segment.end);
            srt += `${index + 1}\n${start} --> ${end}\n${segment.text.trim()}\n\n`;
        });
        return srt;
    }

    formatSRTTime(seconds) {
        const hours = Math.floor(seconds / 3600);
        const minutes = Math.floor((seconds % 3600) / 60);
        const secs = Math.floor(seconds % 60);
        const ms = Math.floor((seconds % 1) * 1000);
        return `${String(hours).padStart(2, '0')}:${String(minutes).padStart(2, '0')}:${String(secs).padStart(2, '0')},${String(ms).padStart(3, '0')}`;
    }
}

// Initialize app when DOM is ready
document.addEventListener('DOMContentLoaded', () => {
    window.app = new WhisperApp();
});
