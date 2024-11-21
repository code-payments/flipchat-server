-- CreateTable
CREATE TABLE "flipchat_pushtokens" (
    "userId" TEXT NOT NULL,
    "appInstallId" TEXT NOT NULL,
    "token" TEXT NOT NULL,
    "type" INTEGER NOT NULL DEFAULT 0,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "flipchat_pushtokens_pkey" PRIMARY KEY ("userId","appInstallId")
);
