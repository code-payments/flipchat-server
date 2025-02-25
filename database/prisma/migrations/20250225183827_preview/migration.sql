-- CreateTable
CREATE TABLE "flipchat_previews" (
    "id" TEXT NOT NULL,
    "originalUrl" TEXT NOT NULL,
    "contentType" SMALLINT NOT NULL DEFAULT 0,
    "moderation" SMALLINT NOT NULL DEFAULT 0,
    "url" TEXT NOT NULL,
    "title" TEXT NOT NULL,
    "description" TEXT NOT NULL,
    "imageUrl" TEXT NOT NULL,
    "imageHash" TEXT NOT NULL,
    "imageWidth" INTEGER NOT NULL,
    "imageHeight" INTEGER NOT NULL,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "flipchat_previews_pkey" PRIMARY KEY ("id")
);
