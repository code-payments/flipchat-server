/*
  Warnings:

  - The primary key for the `flipchat_iap` table will be changed. If it partially fails, the table could be left without primary key constraint.
  - You are about to drop the column `receipt` on the `flipchat_iap` table. All the data in the column will be lost.
  - Added the required column `receiptId` to the `flipchat_iap` table without a default value. This is not possible if the table is not empty.

*/
-- AlterTable
ALTER TABLE "flipchat_iap" DROP CONSTRAINT "flipchat_iap_pkey",
DROP COLUMN "receipt",
ADD COLUMN     "receiptId" TEXT NOT NULL,
ADD CONSTRAINT "flipchat_iap_pkey" PRIMARY KEY ("receiptId");
