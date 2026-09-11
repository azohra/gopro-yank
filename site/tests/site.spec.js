import { expect, test } from "@playwright/test";

for (const [platform, installer] of [["macOS", "install.sh"], ["Windows", "install.ps1"], ["Linux", "install.sh"]]) {
  test(`shows and copies the ${platform} installation command`, async ({ page, context }) => {
    await page.addInitScript((platform) => {
      Object.defineProperty(navigator, "userAgentData", { value: { platform } });
    }, platform);
    await context.grantPermissions(["clipboard-read", "clipboard-write"]);
    await page.goto("/");
    await expect(page.locator("[data-install-title]")).toContainText(platform);
    const command = page.locator("[data-install-command]");
    await expect(command).toContainText(`/${installer}`);
    await expect(page.locator("[data-installer-source]")).toHaveAttribute("href", new RegExp(`/site/public/${installer}$`));
    await page.locator("[data-copy-command]").click();
    await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(await command.textContent());
    await page.locator("summary").click();
    const brew = page.locator("[data-copy-value]");
    await brew.click();
    await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(await brew.getAttribute("data-copy-value"));
  });
}

for (const width of [390, 1440]) {
  test(`serves the built page and assets at ${width}px`, async ({ page, request }) => {
    await page.setViewportSize({ width, height: 900 });
    const errors = [];
    page.on("pageerror", (error) => errors.push(error.message));
    page.on("response", (response) => {
      if (response.url().startsWith("http://127.0.0.1:4173") && !response.ok()) errors.push(response.url());
    });
    const response = await page.goto("/");
    expect(response.status()).toBe(200);
    expect(response.headers()["content-security-policy"]).toContain("frame-ancestors 'none'");
    await expect(page.locator("[data-copy-command]")).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    const anchors = await page.locator('a[href^="#"]').evaluateAll((links) => links.map((link) => link.hash.slice(1)));
    for (const id of anchors) expect(await page.evaluate((id) => Boolean(document.getElementById(id)), id)).toBe(true);
    expect(errors).toEqual([]);
    for (const path of ["/install.sh", "/install.ps1", "/og.png"]) expect((await request.get(path)).status()).toBe(200);
    for (const path of ["/not-a-page", "/mise.toml", "/tests/site.spec.js", "/wrangler.jsonc"]) expect((await request.get(path)).status()).toBe(404);
    expect((await request.get("/404.html")).status()).toBe(200);
  });
}
