.PHONY: all build clean whisper-cpp

WHISPER_CPP_VERSION = v1.8.2
WHISPER_CPP_DIR = ../whisper.cpp
WHISPER_BUILD_DIR = $(WHISPER_CPP_DIR)/build

CGO_CFLAGS = -I$(WHISPER_CPP_DIR)/include -I$(WHISPER_CPP_DIR)/ggml/include
CGO_LDFLAGS = -L$(WHISPER_BUILD_DIR)/src -lwhisper \
							-L$(WHISPER_BUILD_DIR)/ggml/src -lggml \
							-L$(WHISPER_BUILD_DIR)/ggml/src/ggml-metal -lggml-metal \
							-L$(WHISPER_BUILD_DIR)/ggml/src/ggml-blas -lggml-blas \
							-Wl,-rpath,$(WHISPER_BUILD_DIR)/src \
							-Wl,-rpath,$(WHISPER_BUILD_DIR)/ggml/src \
							-Wl,-rpath,$(WHISPER_BUILD_DIR)/ggml/src/ggml-metal \
							-Wl,-rpath,$(WHISPER_BUILD_DIR)/ggml/src/ggml-blas \
							-framework Accelerate -framework Metal -framework Foundation

all: whisper-cpp build

whisper-cpp:
	@if [ ! -d "$(WHISPER_CPP_DIR)" ]; then \
		echo "Cloning whisper.cpp $(WHISPER_CPP_VERSION)..."; \
		cd .. && git clone --depth 1 --branch $(WHISPER_CPP_VERSION) https://github.com/ggerganov/whisper.cpp.git; \
	fi
	@if [ ! -f "$(WHISPER_BUILD_DIR)/src/libwhisper.dylib" ]; then \
		echo "Building whisper.cpp..."; \
		cd $(WHISPER_CPP_DIR) && make -j$$(sysctl -n hw.ncpu); \
	fi

build: whisper-cpp
	@echo "Building transcriber-pro..."
	$(eval VERSION ?= dev)
	$(eval ABS_WHISPER_DIR := $(shell cd $(WHISPER_CPP_DIR) && pwd))
	$(eval ABS_BUILD_DIR := $(ABS_WHISPER_DIR)/build)
	CGO_CFLAGS="-I$(ABS_WHISPER_DIR)/include -I$(ABS_WHISPER_DIR)/ggml/include" \
	CGO_LDFLAGS="-L$(ABS_BUILD_DIR)/src -lwhisper \
	             -L$(ABS_BUILD_DIR)/ggml/src -lggml \
	             -L$(ABS_BUILD_DIR)/ggml/src/ggml-metal -lggml-metal \
	             -L$(ABS_BUILD_DIR)/ggml/src/ggml-blas -lggml-blas \
	             -Wl,-rpath,$(ABS_BUILD_DIR)/src \
	             -Wl,-rpath,$(ABS_BUILD_DIR)/ggml/src \
	             -Wl,-rpath,$(ABS_BUILD_DIR)/ggml/src/ggml-metal \
	             -Wl,-rpath,$(ABS_BUILD_DIR)/ggml/src/ggml-blas \
	             -framework Accelerate -framework Metal -framework Foundation" \
	go build -ldflags "-X main.Version=$(VERSION)" -o transcriber-pro

clean:
	rm -f transcriber-pro
	rm -rf $(WHISPER_CPP_DIR)
