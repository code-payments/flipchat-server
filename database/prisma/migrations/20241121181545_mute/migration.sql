/*
  Warnings:

  - You are about to drop the column `hasMuted` on the `flipchat_members` table. All the data in the column will be lost.

*/
-- AlterTable
ALTER TABLE "flipchat_members" DROP COLUMN "hasMuted",
ADD COLUMN     "isMuted" BOOLEAN NOT NULL DEFAULT false;
