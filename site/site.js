const downloads = {
  macos: {
    arm64: "https://github.com/azohra/gopro-yank/releases/latest/download/gopro-yank_darwin_arm64.tar.gz",
    amd64: "https://github.com/azohra/gopro-yank/releases/latest/download/gopro-yank_darwin_amd64.tar.gz",
    label: "Download for macOS",
  },
  windows: {
    arm64: "https://github.com/azohra/gopro-yank/releases/latest/download/gopro-yank_windows_arm64.zip",
    amd64: "https://github.com/azohra/gopro-yank/releases/latest/download/gopro-yank_windows_amd64.zip",
    label: "Download for Windows",
  },
  linux: {
    arm64: "https://github.com/azohra/gopro-yank/releases/latest/download/gopro-yank_linux_arm64.tar.gz",
    amd64: "https://github.com/azohra/gopro-yank/releases/latest/download/gopro-yank_linux_amd64.tar.gz",
    label: "Download for Linux",
  },
};

function detectPlatform() {
  const platform = navigator.userAgentData?.platform || navigator.platform || navigator.userAgent;
  const value = platform.toLowerCase();
  if (value.includes("win")) return "windows";
  if (value.includes("mac")) return "macos";
  if (value.includes("linux") && !value.includes("android")) return "linux";
  return null;
}

function architectureFrom(value) {
  const normalized = String(value || "").toLowerCase();
  if (normalized.includes("arm") || normalized.includes("aarch64")) return "arm64";
  if (normalized.includes("x86") || normalized.includes("x64") || normalized.includes("amd64")) return "amd64";
  return null;
}

function setPrimaryDownload(platform, architecture) {
  const primary = document.querySelector("[data-primary-download]");
  const label = document.querySelector("[data-download-label]");
  const meta = document.querySelector("[data-download-meta]");
  if (!primary || !label || !meta || !downloads[platform]) return;

  const fallback = platform === "macos" ? "arm64" : "amd64";
  primary.href = downloads[platform][architecture || fallback];
  label.textContent = downloads[platform].label;
  meta.textContent = "Latest release · ready for this computer";
}

async function configureDownload() {
  const platform = detectPlatform();
  if (!platform) return;

  let architecture = architectureFrom(navigator.userAgent);
  if (navigator.userAgentData?.getHighEntropyValues) {
    try {
      const details = await navigator.userAgentData.getHighEntropyValues(["architecture", "bitness"]);
      architecture = architectureFrom(details.architecture) || architecture;
    } catch {
      // The common architecture for each platform remains a safe fallback.
    }
  }
  setPrimaryDownload(platform, architecture);
}

configureDownload();
