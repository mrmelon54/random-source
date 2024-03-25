CREATE TABLE repositories
(
    id         INTEGER UNIQUE PRIMARY KEY AUTOINCREMENT,
    name       TEXT UNIQUE NOT NULL,
    branch     TEXT        NOT NULL,
    updated_at DATETIME    NOT NULL,
    indexed_at DATETIME    NOT NULL,
    had_error  BOOLEAN     NOT NULL
);

CREATE TABLE files_index
(
    id            INTEGER UNIQUE PRIMARY KEY AUTOINCREMENT,
    repository_id INTEGER NOT NULL REFERENCES repositories (id),
    path          TEXT    NOT NULL,
    lines         INTEGER NOT NULL
);
