-- CreateTable
CREATE TABLE "flipchat_chats" (
    "id" TEXT NOT NULL,
    "title" TEXT NOT NULL,
    "roomNumber" INTEGER NOT NULL,
    "coverCharge" INTEGER NOT NULL,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "flipchat_chats_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "flipchat_members" (
    "id" TEXT NOT NULL,
    "chatId" TEXT NOT NULL,
    "userId" TEXT NOT NULL,
    "addedById" TEXT NOT NULL,
    "isHost" BOOLEAN NOT NULL,
    "numUnread" INTEGER NOT NULL,
    "hasMuted" BOOLEAN NOT NULL,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "flipchat_members_pkey" PRIMARY KEY ("id")
);

-- AddForeignKey
ALTER TABLE "flipchat_members" ADD CONSTRAINT "flipchat_members_chatId_fkey" FOREIGN KEY ("chatId") REFERENCES "flipchat_chats"("id") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "flipchat_members" ADD CONSTRAINT "flipchat_members_userId_fkey" FOREIGN KEY ("userId") REFERENCES "flipchat_users"("id") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "flipchat_members" ADD CONSTRAINT "flipchat_members_addedById_fkey" FOREIGN KEY ("addedById") REFERENCES "flipchat_users"("id") ON DELETE RESTRICT ON UPDATE CASCADE;
