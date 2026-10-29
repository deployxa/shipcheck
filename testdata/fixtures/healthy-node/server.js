const express = require("express");
const PORT = process.env.PORT || 3000;
const API_URL = process.env.API_URL || "http://localhost:9000";
const app = express();
app.get("/healthz", (req, res) => res.json({ ok: true }));
const server = app.listen(PORT, () => console.log("listening on " + PORT));
process.on("SIGTERM", () => {
  server.close(() => process.exit(0));
});
