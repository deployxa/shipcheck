import os
from flask import Flask

app = Flask(__name__)

@app.route("/health")
def health():
    return "ok"

if __name__ == "__main__":
    port = int(os.environ.get("PORT", "8000"))
    app.run(port=port)
