-- CreateTable
CREATE TABLE "flipchat_promotedchats" (
    "chatId" TEXT NOT NULL,
    "topic" TEXT NOT NULL,
    "score" INTEGER NOT NULL DEFAULT 0,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "flipchat_promotedchats_pkey" PRIMARY KEY ("chatId","topic")
);

-- AddForeignKey
ALTER TABLE "flipchat_promotedchats" ADD CONSTRAINT "flipchat_promotedchats_chatId_fkey" FOREIGN KEY ("chatId") REFERENCES "flipchat_chats"("id") ON DELETE RESTRICT ON UPDATE CASCADE;
