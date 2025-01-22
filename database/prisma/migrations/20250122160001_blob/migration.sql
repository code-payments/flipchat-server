-- CreateTable
CREATE TABLE "flipchat_blob" (
    "id" BYTEA NOT NULL,
    "userId" TEXT NOT NULL,
    "type" SMALLINT NOT NULL,
    "s3Url" TEXT NOT NULL,
    "size" INTEGER NOT NULL,
    "metadata" BYTEA NOT NULL,
    "flagged" BOOLEAN NOT NULL DEFAULT false,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "flipchat_blob_pkey" PRIMARY KEY ("id")
);
