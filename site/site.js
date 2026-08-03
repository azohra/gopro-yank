const installers = {
  macos: {
    title: "Install on macOS",
    help: "Open Terminal, paste this command, then press Return.",
    command: 'curl -fsSL https://gopro-yank.azohra.com/install.sh | sh && "$HOME/.local/bin/gopro-yank"',
    result: "The script selects the right Mac build, verifies its checksum, installs it, and opens GoPro Yank.",
    source: "https://github.com/azohra/gopro-yank/blob/main/site/install.sh",
  },
  windows: {
    title: "Install on Windows",
    help: "Open PowerShell, paste this command, then press Enter.",
    command: "irm https://gopro-yank.azohra.com/install.ps1 | iex",
    result: "The script selects the right Windows build, verifies its checksum, installs it, and opens GoPro Yank.",
    source: "https://github.com/azohra/gopro-yank/blob/main/site/install.ps1",
  },
  linux: {
    title: "Install on Linux",
    help: "Open a terminal, paste this command, then press Enter.",
    command: 'curl -fsSL https://gopro-yank.azohra.com/install.sh | sh && "$HOME/.local/bin/gopro-yank"',
    result: "The script selects the right Linux build, verifies its checksum, installs it, and opens GoPro Yank.",
    source: "https://github.com/azohra/gopro-yank/blob/main/site/install.sh",
  },
};

function detectPlatform() {
  const platform = navigator.userAgentData?.platform || navigator.platform || navigator.userAgent;
  const value = platform.toLowerCase();
  if (value.includes("win")) return "windows";
  if (value.includes("mac")) return "macos";
  if (value.includes("linux") && !value.includes("android")) return "linux";
  return "macos";
}

function configureInstaller() {
  const installer = installers[detectPlatform()];
  const title = document.querySelector("[data-install-title]");
  const help = document.querySelector("[data-install-help]");
  const command = document.querySelector("[data-install-command]");
  const result = document.querySelector("[data-install-result]");
  const source = document.querySelector("[data-installer-source]");
  if (!title || !help || !command || !result || !source) return;

  title.textContent = installer.title;
  help.textContent = installer.help;
  command.textContent = installer.command;
  result.textContent = installer.result;
  source.href = installer.source;
}

async function copyText(button, value) {
  try {
    await navigator.clipboard.writeText(value);
  } catch {
    const textarea = document.createElement("textarea");
    textarea.value = value;
    textarea.style.position = "fixed";
    textarea.style.opacity = "0";
    document.body.append(textarea);
    textarea.select();
    document.execCommand("copy");
    textarea.remove();
  }
  const original = button.textContent;
  button.textContent = "Copied";
  window.setTimeout(() => { button.textContent = original; }, 1800);
}

function configureCopyButtons() {
  const primary = document.querySelector("[data-copy-command]");
  const command = document.querySelector("[data-install-command]");
  if (primary && command) {
    primary.addEventListener("click", () => copyText(primary, command.textContent));
  }
  document.querySelectorAll("[data-copy-value]").forEach((button) => {
    button.addEventListener("click", () => copyText(button, button.dataset.copyValue));
  });
}

configureInstaller();
configureCopyButtons();
