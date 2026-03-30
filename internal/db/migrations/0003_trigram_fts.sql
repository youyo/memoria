-- Migration 0003: FTS5 トークナイザーを unicode61 から trigram に変更。
-- trigram は日本語等 CJK 文字の部分文字列マッチを可能にする。

-- 1. 既存トリガー削除
DROP TRIGGER IF EXISTS chunks_fts_insert;
DROP TRIGGER IF EXISTS chunks_fts_delete;
DROP TRIGGER IF EXISTS chunks_fts_update;

-- 2. 既存 FTS テーブル削除・再作成（trigram）
DROP TABLE IF EXISTS chunks_fts;
CREATE VIRTUAL TABLE chunks_fts USING fts5(
    content,
    summary,
    keywords,
    content='chunks',
    content_rowid='rowid',
    tokenize='trigram'
);

-- 3. トリガー再作成（0001_initial.sql と同一ロジック）
CREATE TRIGGER chunks_fts_insert AFTER INSERT ON chunks BEGIN
    INSERT INTO chunks_fts(rowid, content, summary, keywords)
    VALUES (new.rowid, new.content, COALESCE(new.summary, ''), COALESCE(new.keywords_json, ''));
END;

CREATE TRIGGER chunks_fts_delete AFTER DELETE ON chunks BEGIN
    INSERT INTO chunks_fts(chunks_fts, rowid, content, summary, keywords)
    VALUES ('delete', old.rowid, old.content, COALESCE(old.summary, ''), COALESCE(old.keywords_json, ''));
END;

CREATE TRIGGER chunks_fts_update AFTER UPDATE ON chunks BEGIN
    INSERT INTO chunks_fts(chunks_fts, rowid, content, summary, keywords)
    VALUES ('delete', old.rowid, old.content, COALESCE(old.summary, ''), COALESCE(old.keywords_json, ''));
    INSERT INTO chunks_fts(rowid, content, summary, keywords)
    VALUES (new.rowid, new.content, COALESCE(new.summary, ''), COALESCE(new.keywords_json, ''));
END;

-- 4. 既存データからインデックス再構築
-- chunks テーブルから FTS に既存レコードを挿入する
-- （content= モードの rebuild は chunks カラム名マッピングの都合で使用できないため手動挿入）
INSERT INTO chunks_fts(rowid, content, summary, keywords)
SELECT rowid, content, COALESCE(summary, ''), COALESCE(keywords_json, '')
FROM chunks;
