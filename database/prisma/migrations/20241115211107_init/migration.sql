-- CreateTable
CREATE TABLE "flipchat_users" (
    "id" TEXT NOT NULL,
    "displayName" TEXT,

    CONSTRAINT "flipchat_users_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "flipchat_publickeys" (
    "key" TEXT NOT NULL,
    "userId" TEXT NOT NULL,

    CONSTRAINT "flipchat_publickeys_pkey" PRIMARY KEY ("key")
);

-- AddForeignKey
ALTER TABLE "flipchat_publickeys" ADD CONSTRAINT "flipchat_publickeys_userId_fkey" FOREIGN KEY ("userId") REFERENCES "flipchat_users"("id") ON DELETE RESTRICT ON UPDATE CASCADE;
