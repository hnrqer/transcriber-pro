/**
 * Transcriber Pro - PWA Application with Queue Support
 * Handles server detection, file upload, and transcription queuing
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
        this.queuePollInterval = null;
        this.selectedJobId = null; // Currently selected/viewed job
        this.queueLoaded = false; // Track if queue has been loaded at least once

        // UI elements
        this.elements = {
            connectionStatus: document.getElementById('connectionStatus'),
            statusText: document.querySelector('.status-text'),
            serverInstructions: document.getElementById('serverInstructions'),
            appLayout: document.getElementById('appLayout'),
            uploadSection: document.getElementById('uploadSection'),
            loadingOverlay: document.getElementById('loadingOverlay'),
            processingSection: document.getElementById('processingSection'),
            resultsSection: document.getElementById('resultsSection'),
            queueSection: document.getElementById('queueSection'),

            dropZone: document.getElementById('dropZone'),
            fileInput: document.getElementById('fileInput'),
            languageSelect: document.getElementById('languageSelect'),

            processingFileName: document.getElementById('processingFileName'),
            processingStatus: document.getElementById('processingStatus'),
            progressFill: document.getElementById('progressFill'),
            progressPercent: document.getElementById('progressPercent'),
            etaText: document.getElementById('etaText'),
            etaValue: document.getElementById('etaValue'),

            queueList: document.getElementById('queueList'),
            queueCount: document.getElementById('queueCount'),

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

        // Start queue polling
        this.startQueuePolling();
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

            const files = Array.from(e.dataTransfer.files);
            if (files.length > 0) {
                this.handleFileSelection(files);
            }
        });

        this.elements.fileInput.addEventListener('change', (e) => {
            const files = Array.from(e.target.files);
            if (files.length > 0) {
                this.handleFileSelection(files);
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

    async onCompanionConnected(info) {
        console.log('[WhisperApp] Companion connected:', info);

        // Hide connection status when connected
        this.elements.connectionStatus.style.display = 'none';

        this.elements.serverInstructions.style.display = 'none';
        this.elements.appLayout.style.display = 'flex';

        // Show loading overlay until first queue fetch completes
        if (!this.queueLoaded) {
            this.elements.loadingOverlay.style.display = 'flex';
        }
    }

    onCompanionDisconnected() {
        console.log('[WhisperApp] Companion not found');

        this.serverUrl = null;

        // Show connection status when disconnected
        this.elements.connectionStatus.style.display = 'flex';
        this.elements.connectionStatus.className = 'connection-status disconnected';
        this.elements.statusText.textContent = 'Companion not running';

        this.elements.serverInstructions.style.display = 'block';
        this.elements.uploadSection.style.display = 'none';
        this.elements.processingSection.style.display = 'none';
        this.elements.resultsSection.style.display = 'none';
        this.elements.queueSection.style.display = 'none';
    }

    async handleFileSelection(files) {
        console.log('[WhisperApp] Files selected:', files.length);

        if (!this.serverUrl) {
            alert('Companion not connected. Please start the server app first.');
            return;
        }

        // Add files to queue and upload them
        for (const file of files) {
            await this.uploadFile(file);
        }

        // Show queue section
        this.elements.queueSection.style.display = 'block';
    }

    async uploadFile(file) {
        try {
            const formData = new FormData();
            formData.append('audio', file);

            const language = this.elements.languageSelect.value;
            if (language) {
                formData.append('language', language);
            }

            console.log('[WhisperApp] Uploading:', file.name);

            // Upload file and get job ID
            const response = await fetch(`${this.serverUrl}/transcribe`, {
                method: 'POST',
                body: formData
            });

            if (!response.ok) {
                const error = await response.json();
                throw new Error(error.error || 'Failed to start transcription');
            }

            const { job_id } = await response.json();
            console.log('[WhisperApp] Job created:', job_id, file.name);

            // Store job info
            this.activeJobs.set(job_id, {
                id: job_id,
                fileName: file.name,
                status: 'queued',
                progress: 0
            });

        } catch (error) {
            console.error('[WhisperApp] Upload error:', error);
            alert(`Upload failed for ${file.name}: ${error.message}`);
        }
    }

    startQueuePolling() {
        // Poll queue status every second
        this.queuePollInterval = setInterval(async () => {
            await this.updateQueueStatus();
        }, 1000);
    }

    async updateQueueStatus() {
        try {
            const response = await fetch(`${this.serverUrl}/queue`);
            if (!response.ok) return;

            const data = await response.json();
            this.renderQueue(data.queue || [], data.completed || []);

        } catch (error) {
            console.error('[WhisperApp] Queue polling error:', error);
        }
    }

    renderQueue(queue, completed) {
        if (!this.elements.queueList) return;

        // Hide loading overlay and mark queue as loaded
        if (!this.queueLoaded) {
            this.queueLoaded = true;
            this.elements.loadingOverlay.style.display = 'none';
        }

        this.elements.queueCount.textContent = queue.length;

        // Remove empty placeholder if it exists
        const emptyPlaceholder = this.elements.queueList.querySelector('.queue-empty');
        if (emptyPlaceholder && (queue.length > 0 || completed.length > 0)) {
            emptyPlaceholder.remove();
        }

        // Show empty placeholder if no jobs
        if (queue.length === 0 && completed.length === 0) {
            if (!emptyPlaceholder) {
                this.elements.queueList.innerHTML = `
                    <div class="queue-empty">
                        <p>No jobs in queue</p>
                        <p class="queue-empty-hint">Upload files to start transcribing</p>
                    </div>
                `;
            }
            return;
        }

        // Get existing items by job ID for comparison
        const existingItems = new Map();
        this.elements.queueList.querySelectorAll('.queue-item[data-job-id]').forEach(item => {
            existingItems.set(item.dataset.jobId, item);
        });

        // Track which items we've processed
        const processedIds = new Set();

        // Render active queue items
        queue.forEach((job, index) => {
            processedIds.add(job.ID);
            const existing = existingItems.get(job.ID);

            // If item exists, just update dynamic content (progress, ETA, status)
            if (existing) {
                this.updateQueueItem(existing, job, index);
                return;
            }

            // Create new item if it doesn't exist
            const item = this.createQueueItem(job, index);
            // Insert in correct position
            const allItems = Array.from(this.elements.queueList.querySelectorAll('.queue-item[data-job-id]'));
            if (index < allItems.length) {
                this.elements.queueList.insertBefore(item, allItems[index]);
            } else {
                // Find completed header or append to end
                const completedHeader = this.elements.queueList.querySelector('h3');
                if (completedHeader) {
                    this.elements.queueList.insertBefore(item, completedHeader);
                } else {
                    this.elements.queueList.appendChild(item);
                }
            }
        });

        // Remove items that are no longer in queue
        existingItems.forEach((item, id) => {
            if (!processedIds.has(id) && !item.classList.contains('completed-job')) {
                item.remove();
            }
        });

        // Handle completed section separately (less critical, can rebuild)
        this.renderCompletedSection(completed);
    }

    createQueueItem(job, index) {
        const item = document.createElement('div');
        item.className = `queue-item status-${job.Status}`;
        item.dataset.jobId = job.ID; // Important: track by ID

        const statusBadge = this.getStatusBadge(job.Status);
        const isProcessing = index === 0 && (job.Status === 'processing' || job.Status === 'transcribing');
        const canCancel = job.Status === 'queued' || isProcessing;

        item.innerHTML = `
            <div class="queue-item-header">
                <span class="queue-position">#${index + 1}</span>
                <span class="queue-filename">${job.FileName}</span>
                <span class="queue-status-badge ${statusBadge.class}">${statusBadge.text}</span>
                ${canCancel ? `<button class="cancel-job-btn" data-job-id="${job.ID}" title="Cancel">✕</button>` : ''}
            </div>
            ${isProcessing ? `
                <div class="queue-progress">
                    <div class="progress-bar-small">
                        <div class="progress-fill-small" style="width: ${job.Progress}%"></div>
                    </div>
                    <span class="queue-progress-text">${Math.round(job.Progress)}%</span>
                </div>
                ${job.ETA ? `<div class="queue-eta">ETA: ${job.ETA}</div>` : ''}
            ` : `
                <div class="queue-message">${job.Message}${job.ETA ? ` · ETA: ${job.ETA}` : ''}</div>
            `}
        `;

        // Add cancel button handler
        const cancelBtn = item.querySelector('.cancel-job-btn');
        if (cancelBtn) {
            cancelBtn.addEventListener('click', async (e) => {
                e.stopPropagation();
                const jobId = e.target.dataset.jobId;
                // Check job status at click time, not creation time
                // Always use killJob for actively processing jobs to ensure worker is killed
                await this.killJob(jobId);
            });
        }

        return item;
    }

    updateQueueItem(item, job, index) {
        // Update position
        const posElem = item.querySelector('.queue-position');
        if (posElem) posElem.textContent = `#${index + 1}`;

        // Update status badge
        const statusBadge = this.getStatusBadge(job.Status);
        const badgeElem = item.querySelector('.queue-status-badge');
        if (badgeElem) {
            badgeElem.className = `queue-status-badge ${statusBadge.class}`;
            badgeElem.textContent = statusBadge.text;
        }

        // Update progress if it exists
        const progressBar = item.querySelector('.progress-fill-small');
        if (progressBar) {
            progressBar.style.width = `${job.Progress}%`;
        }

        const progressText = item.querySelector('.queue-progress-text');
        if (progressText) {
            progressText.textContent = `${Math.round(job.Progress)}%`;
        }

        // Update ETA
        const etaElem = item.querySelector('.queue-eta');
        if (etaElem && job.ETA) {
            etaElem.textContent = `ETA: ${job.ETA}`;
        } else if (etaElem && !job.ETA) {
            etaElem.remove();
        }

        // Update message if no progress bar
        const msgElem = item.querySelector('.queue-message');
        if (msgElem) {
            msgElem.textContent = `${job.Message}${job.ETA ? ` · ETA: ${job.ETA}` : ''}`;
        }
    }

    renderCompletedSection(completed) {
        // Remove old completed/failed sections (including headers and items)
        const oldSections = this.elements.queueList.querySelectorAll('.section-header, h3, .completed-job, .failed-job');
        oldSections.forEach(el => el.remove());

        // Separate completed and failed jobs
        const successfulJobs = completed.filter(job => job.Status === 'completed');
        const failedJobs = completed.filter(job => job.Status === 'failed');

        // Render successful completed jobs
        if (successfulJobs.length > 0) {
            const completedHeader = document.createElement('h3');
            completedHeader.className = 'section-header';
            completedHeader.textContent = `✅ Completed (${successfulJobs.length})`;
            completedHeader.style.marginTop = '20px';
            completedHeader.style.marginBottom = '10px';
            this.elements.queueList.appendChild(completedHeader);

            successfulJobs.forEach((job) => {
                const item = this.createCompletedJobItem(job, 'completed-job');
                this.elements.queueList.appendChild(item);
            });

            // Auto-show the first completed job (only if nothing selected)
            if (!this.selectedJobId && successfulJobs.length > 0 && successfulJobs[0].Result) {
                this.selectedJobId = successfulJobs[0].ID;
                this.showResults(successfulJobs[0].Result, successfulJobs[0].FileName);
            }
        }

        // Render failed jobs
        if (failedJobs.length > 0) {
            const failedHeaderContainer = document.createElement('div');
            failedHeaderContainer.className = 'section-header';
            failedHeaderContainer.style.display = 'flex';
            failedHeaderContainer.style.justifyContent = 'space-between';
            failedHeaderContainer.style.alignItems = 'center';
            failedHeaderContainer.style.marginTop = '20px';
            failedHeaderContainer.style.marginBottom = '10px';

            const failedHeader = document.createElement('h3');
            failedHeader.textContent = `❌ Failed (${failedJobs.length})`;
            failedHeader.style.margin = '0';

            const clearBtn = document.createElement('button');
            clearBtn.textContent = 'Clear Failed';
            clearBtn.className = 'clear-failed-btn';
            clearBtn.addEventListener('click', async () => {
                await this.clearCompleted();
            });

            failedHeaderContainer.appendChild(failedHeader);
            failedHeaderContainer.appendChild(clearBtn);
            this.elements.queueList.appendChild(failedHeaderContainer);

            failedJobs.forEach((job) => {
                const item = this.createCompletedJobItem(job, 'failed-job');
                this.elements.queueList.appendChild(item);
            });
        }
    }

    createCompletedJobItem(job, className) {
        const item = document.createElement('div');
        item.className = `queue-item ${className} status-${job.Status}`;
        item.style.cursor = job.Status === 'completed' && job.Result ? 'pointer' : 'default';
        item.dataset.jobId = job.ID;

        // Highlight if this is the selected job
        if (this.selectedJobId === job.ID) {
            item.classList.add('selected');
        }

        const statusBadge = this.getStatusBadge(job.Status);
        const icon = job.Status === 'completed' ? '✓' : '✗';

        item.innerHTML = `
            <div class="queue-item-header">
                <span class="queue-position">${icon}</span>
                <span class="queue-filename">${job.FileName}</span>
                <span class="queue-status-badge ${statusBadge.class}">${statusBadge.text}</span>
            </div>
            ${job.Error ? `<div class="queue-error">${job.Error}</div>` : ''}
        `;

        // Click to view results for successful jobs
        if (job.Status === 'completed' && job.Result) {
            item.addEventListener('click', () => {
                this.selectedJobId = job.ID;
                this.showResults(job.Result, job.FileName);
                // Re-render queue with current data (will be refetched on next poll)
                this.updateQueueStatus();
            });
        }

        return item;
    }

    getStatusBadge(status) {
        const badges = {
            'queued': { class: 'badge-queued', text: 'Queued' },
            'processing': { class: 'badge-processing', text: 'Processing' },
            'transcribing': { class: 'badge-processing', text: 'Transcribing' },
            'completed': { class: 'badge-completed', text: 'Completed' },
            'failed': { class: 'badge-failed', text: 'Failed' }
        };
        return badges[status] || { class: '', text: status };
    }

    showResults(result, fileName) {
        console.log('[WhisperApp] Showing results:', result, fileName);

        this.elements.resultsSection.style.display = 'block';
        this.transcriptionResult = result;

        // Update the results header to show which file
        const resultsHeader = this.elements.resultsSection.querySelector('.results-header h2');
        if (resultsHeader && fileName) {
            resultsHeader.textContent = `✅ ${fileName}`;
        }

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

    async cancelJob(jobId) {
        try {
            const response = await fetch(`${this.serverUrl}/cancel-job/${jobId}`, {
                method: 'POST'
            });

            if (!response.ok) {
                throw new Error('Failed to cancel job');
            }

            console.log(`[WhisperApp] Job ${jobId} cancelled`);
        } catch (error) {
            console.error('[WhisperApp] Failed to cancel job:', error);
        }
    }

    async killJob(jobId) {
        try {
            const response = await fetch(`${this.serverUrl}/kill-job/${jobId}`, {
                method: 'POST'
            });

            if (!response.ok) {
                throw new Error('Failed to kill job');
            }

            console.log(`[WhisperApp] Job ${jobId} killed`);
        } catch (error) {
            console.error('[WhisperApp] Failed to kill job:', error);
        }
    }

    async resetToUpload() {
        // Clear all jobs (both queued and completed) from backend
        try {
            await fetch(`${this.serverUrl}/clear-all`, {
                method: 'POST'
            });
        } catch (error) {
            console.error('[WhisperApp] Failed to clear all jobs:', error);
        }

        this.currentFile = null;
        this.transcriptionResult = null;
        this.selectedJobId = null;

        this.elements.resultsSection.style.display = 'none';
        this.elements.queueSection.style.display = 'none';
        this.elements.fileInput.value = '';

        // Clear the queue list
        if (this.elements.queueList) {
            this.elements.queueList.innerHTML = '';
        }
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

    async clearCompleted() {
        try {
            const response = await fetch(`${this.serverUrl}/clear-completed`, {
                method: 'POST'
            });

            if (!response.ok) {
                throw new Error('Failed to clear completed jobs');
            }

            console.log('[WhisperApp] Cleared failed/completed jobs');
        } catch (error) {
            console.error('[WhisperApp] Failed to clear completed jobs:', error);
        }
    }
}

// Initialize app when DOM is ready
document.addEventListener('DOMContentLoaded', () => {
    window.app = new WhisperApp();
});
