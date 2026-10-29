const db = require("better-sqlite3")("./data/app.db");
const UPLOAD_DIR = process.env.UPLOAD_DIR || "./uploads";
console.log(db, UPLOAD_DIR);
