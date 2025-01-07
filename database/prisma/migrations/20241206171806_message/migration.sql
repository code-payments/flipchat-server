-- AlterTable
ALTER TABLE "flipchat_messages" 
ADD COLUMN     "contentType" SMALLINT NOT NULL DEFAULT 0,
ADD COLUMN     "version" SMALLINT NOT NULL DEFAULT 0;
