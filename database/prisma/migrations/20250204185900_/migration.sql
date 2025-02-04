-- CreateTable
CREATE TABLE "flipchat_x_users" (
    "id" TEXT NOT NULL,
    "username" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "description" TEXT NOT NULL,
    "profilePicUrl" TEXT NOT NULL,
    "followerCount" INTEGER NOT NULL DEFAULT 0,
    "verifiedType" SMALLINT NOT NULL DEFAULT 0,
    "accessToken" TEXT NOT NULL,
    "userId" TEXT NOT NULL,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "flipchat_x_users_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE UNIQUE INDEX "flipchat_x_users_username_key" ON "flipchat_x_users"("username");

-- CreateIndex
CREATE UNIQUE INDEX "flipchat_x_users_userId_key" ON "flipchat_x_users"("userId");

-- AddForeignKey
ALTER TABLE "flipchat_x_users" ADD CONSTRAINT "flipchat_x_users_userId_fkey" FOREIGN KEY ("userId") REFERENCES "flipchat_users"("id") ON DELETE RESTRICT ON UPDATE CASCADE;
