/*
  Warnings:

  - Added the required column `updatedAt` to the `flipchat_publickeys` table without a default value. This is not possible if the table is not empty.
  - Added the required column `updatedAt` to the `flipchat_users` table without a default value. This is not possible if the table is not empty.

*/
-- AlterTable
ALTER TABLE "flipchat_publickeys" ADD COLUMN     "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
ADD COLUMN     "updatedAt" TIMESTAMP(3) NOT NULL;

-- AlterTable
ALTER TABLE "flipchat_users" ADD COLUMN     "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
ADD COLUMN     "updatedAt" TIMESTAMP(3) NOT NULL;

-- CreateTable
CREATE TABLE "flipchat_intents" (
    "id" TEXT NOT NULL,
    "isFulfilled" BOOLEAN NOT NULL,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "flipchat_intents_pkey" PRIMARY KEY ("id")
);
