-- 画像メタデータ取得で14GBの image_objects を読まないよう、
-- image_object_locations に不変の owner_user_id / media_type を持たせる。
ALTER TABLE image_object_locations
  ADD COLUMN IF NOT EXISTS owner_user_id varchar(36);
ALTER TABLE image_object_locations
  ADD COLUMN IF NOT EXISTS media_type text;

UPDATE image_object_locations l
SET owner_user_id = i.owner_user_id,
    media_type = i.media_type
FROM image_objects i
WHERE i.id = l.id
  AND i.created_at = l.created_at
  AND (l.owner_user_id IS NULL OR l.media_type IS NULL);

ALTER TABLE image_object_locations
  ALTER COLUMN owner_user_id SET NOT NULL;
ALTER TABLE image_object_locations
  ALTER COLUMN media_type SET NOT NULL;
