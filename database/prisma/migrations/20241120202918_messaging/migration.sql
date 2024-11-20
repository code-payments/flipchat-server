-- CreateTable
CREATE TABLE "flipchat_messages" (
    "id" TEXT NOT NULL,
    "chatId" TEXT NOT NULL,
    "senderId" TEXT NOT NULL,
    "content" BYTEA NOT NULL,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "flipchat_messages_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "flipchat_pointers" (
    "chatId" TEXT NOT NULL,
    "userId" TEXT NOT NULL,
    "type" INTEGER NOT NULL DEFAULT 0,
    "value" TEXT NOT NULL,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "flipchat_pointers_pkey" PRIMARY KEY ("chatId","userId","type")
);
