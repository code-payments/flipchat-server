-- CreateTable
CREATE TABLE "flipchat_iap" (
    "receipt" TEXT NOT NULL,
    "platform" SMALLINT NOT NULL DEFAULT 0,
    "userId" TEXT NOT NULL,
    "product" SMALLINT NOT NULL DEFAULT 0,
    "state" SMALLINT NOT NULL DEFAULT 0,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "flipchat_iap_pkey" PRIMARY KEY ("receipt")
);
