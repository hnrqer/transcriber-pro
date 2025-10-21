class TranscriberPro < Formula
  desc "Privacy-first audio transcription using Whisper Large-v3"
  homepage "https://github.com/hnrqer/transcriber-pro"
  url "https://github.com/hnrqer/transcriber-pro/archive/refs/tags/v1.0.2.tar.gz"
  sha256 "4ec4a2fd15731883508608f02c8a9ee2c4561d92a568898dd84091a53c9ad705" # Will be updated after creating the tag
  license "MIT"

  depends_on "go" => :build
  depends_on "cmake" => :build
  depends_on "ffmpeg"

  def install
    # Clone and build whisper.cpp
    system "git", "clone", "--depth", "1", "--branch", "v1.8.2",
           "https://github.com/ggerganov/whisper.cpp.git"

    cd "whisper.cpp" do
      system "make", "-j#{Hardware::CPU.cores}"
    end

    # Build Go server
    cd "server" do
      ENV["CGO_CFLAGS"] = "-I#{buildpath}/whisper.cpp/include -I#{buildpath}/whisper.cpp/ggml/include"
      ENV["CGO_LDFLAGS"] = "-L#{buildpath}/whisper.cpp/build/src -lwhisper " \
                           "-L#{buildpath}/whisper.cpp/build/ggml/src -lggml " \
                           "-L#{buildpath}/whisper.cpp/build/ggml/src/ggml-metal -lggml-metal " \
                           "-L#{buildpath}/whisper.cpp/build/ggml/src/ggml-blas -lggml-blas " \
                           "-Wl,-rpath,#{lib} " \
                           "-framework Accelerate -framework Metal -framework Foundation"

      system "go", "build", "-ldflags", "-X main.Version=#{version}", "-o", "transcriber-pro"
      bin.install "transcriber-pro"

      # Install static files
      pkgshare.install "static"
    end

    # Install whisper.cpp libraries
    lib.install Dir["whisper.cpp/build/src/*.dylib"]
    lib.install Dir["whisper.cpp/build/ggml/src/*.dylib"]
    lib.install Dir["whisper.cpp/build/ggml/src/ggml-metal/*.dylib"]
    lib.install Dir["whisper.cpp/build/ggml/src/ggml-blas/*.dylib"]
  end

  def caveats
    <<~EOS
      Transcriber Pro has been installed!

      Run with:
        transcriber-pro

      The web UI will open automatically at http://localhost:8456

      On first run, the Whisper large-v3 model (~3GB) will download
      automatically to ~/.cache/whisper
    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/transcriber-pro --version")
  end
end
