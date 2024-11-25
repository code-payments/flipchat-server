
-- AlterTable
ALTER TABLE "flipchat_chats" ADD COLUMN "createdBy" TEXT NOT NULL DEFAULT '';

-- Set the createdAt value based on the host or the first ever member.
UPDATE "flipchat_chats"
SET "createdBy" = COALESCE(
    -- Find the user marked as host
    (
        SELECT "userId"
        FROM "flipchat_members"
        WHERE "flipchat_members"."chatId" = "flipchat_chats"."id" AND "isHost" = true
        LIMIT 1
    ),
    -- If no host, fallback to the first member (earliest createdAt)
    (
        SELECT "userId"
        FROM "flipchat_members"
        ORDER BY "createdAt" ASC
        LIMIT 1
    )
)
WHERE "createdBy" = '';

-- AlterTable
ALTER TABLE "flipchat_members" RENAME COLUMN "isHost" TO "isMod";