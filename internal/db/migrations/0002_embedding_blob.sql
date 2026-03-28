-- M14: chunk_embeddings と project_embeddings に float32 バイナリ列を追加
-- JSON blob からの高速読み取りパスを提供する（後方互換性のため embedding_json は残す）
ALTER TABLE chunk_embeddings ADD COLUMN embedding_blob BLOB;
ALTER TABLE project_embeddings ADD COLUMN embedding_blob BLOB;
