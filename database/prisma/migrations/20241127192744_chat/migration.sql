/*
  This migration script converts the `id` column in the `flipchat_messages` table
  from a `TEXT` type to a `BYTEA` type.
*/

-- Step 1: Add a temporary column for the new `BYTEA` IDs
ALTER TABLE "flipchat_messages"
ADD COLUMN "id_new" BYTEA;

-- Step 2: Populate the new column by decoding the `id` values
UPDATE "flipchat_messages"
SET "id_new" = decode(SUBSTRING("id" FROM 5), 'base64')
WHERE "id" LIKE 'b64:%';

-- Step 3: Drop the old primary key constraint
ALTER TABLE "flipchat_messages" DROP CONSTRAINT "flipchat_messages_pkey";

-- Step 4: Drop the old `id` column
ALTER TABLE "flipchat_messages" DROP COLUMN "id";

-- Step 5: Rename the temporary column to `id`
ALTER TABLE "flipchat_messages" RENAME COLUMN "id_new" TO "id";

-- Step 6: Add the primary key constraint back to the `id` column
ALTER TABLE "flipchat_messages"
ADD CONSTRAINT "flipchat_messages_pkey" PRIMARY KEY ("id");
