CREATE TABLE IF NOT EXISTS typing_scores (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    wpm INTEGER NOT NULL CHECK (wpm BETWEEN 5 AND 260),
    accuracy REAL NOT NULL CHECK (accuracy BETWEEN 0 AND 100),
    mode TEXT NOT NULL CHECK (mode IN ('15s', '30s', '60s', 'code-go')),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_typing_scores_user_wpm
    ON typing_scores(user_id, wpm DESC);

CREATE INDEX IF NOT EXISTS idx_typing_scores_mode_wpm
    ON typing_scores(mode, wpm DESC);
