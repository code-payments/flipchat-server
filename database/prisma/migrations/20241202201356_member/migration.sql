-- Rename the isMod column
ALTER TABLE "flipchat_members"
RENAME COLUMN "isMod" TO "hasModPermission";

-- Add a new column with the default value of true for existing rows
ALTER TABLE "flipchat_members"
ADD COLUMN "hasSendPermission" BOOLEAN NOT NULL DEFAULT true;

-- Change the default value for future rows to false
ALTER TABLE "flipchat_members"
ALTER COLUMN "hasSendPermission" SET DEFAULT false;