-- CreateTable
CREATE TABLE "flipchat_chats" (
    "id" TEXT NOT NULL,
    "title" TEXT NOT NULL,
    "roomNumber" INTEGER,
    "coverCharge" INTEGER NOT NULL DEFAULT 0,
    "type" INTEGER NOT NULL DEFAULT 0,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "flipchat_chats_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "flipchat_members" (
    "chatId" TEXT NOT NULL,
    "userId" TEXT NOT NULL,
    "addedById" TEXT,
    "isHost" BOOLEAN NOT NULL DEFAULT false,
    "numUnread" INTEGER NOT NULL DEFAULT 0,
    "hasMuted" BOOLEAN NOT NULL DEFAULT false,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "flipchat_members_pkey" PRIMARY KEY ("chatId","userId")
);

-- CreateIndex
CREATE UNIQUE INDEX "flipchat_chats_roomNumber_key" ON "flipchat_chats"("roomNumber");

-- AddForeignKey
ALTER TABLE "flipchat_members" ADD CONSTRAINT "flipchat_members_chatId_fkey" FOREIGN KEY ("chatId") REFERENCES "flipchat_chats"("id") ON DELETE RESTRICT ON UPDATE CASCADE;
