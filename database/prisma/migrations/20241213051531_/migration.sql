-- AlterTable
ALTER TABLE "flipchat_users" ADD COLUMN     "isRegistered" BOOLEAN NOT NULL DEFAULT false;

UPDATE "flipchat_users" SET "isRegistered" = true WHERE "displayName" IS NOT NULL;
