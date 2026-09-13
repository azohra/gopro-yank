import { defineConfig } from "vite";

export default defineConfig({
  build: {
    rolldownOptions: {
      input: ["index.html", "404.html"],
    },
  },
});
