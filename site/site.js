const tabs = [...document.querySelectorAll("[data-platform]")];
const panels = [...document.querySelectorAll("[data-platform-panel]")];

function showPlatform(platform, moveFocus = false) {
  const selected = tabs.find((tab) => tab.dataset.platform === platform);
  if (!selected) return;

  tabs.forEach((tab) => {
    const active = tab === selected;
    tab.setAttribute("aria-selected", String(active));
    tab.tabIndex = active ? 0 : -1;
  });
  panels.forEach((panel) => {
    panel.hidden = panel.dataset.platformPanel !== platform;
  });
  if (moveFocus) selected.focus();
}

function detectedPlatform() {
  const platform = navigator.userAgentData?.platform || navigator.platform || navigator.userAgent;
  const value = platform.toLowerCase();
  if (value.includes("win")) return "windows";
  if (value.includes("mac")) return "macos";
  if (value.includes("linux")) return "linux";
  return null;
}

tabs.forEach((tab, index) => {
  tab.addEventListener("click", () => showPlatform(tab.dataset.platform));
  tab.addEventListener("keydown", (event) => {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
    event.preventDefault();
    const direction = event.key === "ArrowRight" ? 1 : -1;
    const next = (index + direction + tabs.length) % tabs.length;
    showPlatform(tabs[next].dataset.platform, true);
  });
});

const platform = detectedPlatform();
if (platform) showPlatform(platform);

document.querySelectorAll("[data-copy]").forEach((button) => {
  button.addEventListener("click", async () => {
    try {
      await navigator.clipboard.writeText(button.dataset.copy);
      button.textContent = "Copied";
      window.setTimeout(() => { button.textContent = "Copy"; }, 1600);
    } catch {
      button.textContent = "Select the command";
    }
  });
});
