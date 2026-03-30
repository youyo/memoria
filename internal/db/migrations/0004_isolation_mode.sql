-- Migration 0004: プロジェクトレベルの isolation モード追加
ALTER TABLE projects ADD COLUMN isolation_mode TEXT NOT NULL DEFAULT 'normal'
    CHECK (isolation_mode IN ('normal', 'isolated'));
