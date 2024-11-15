-- CreateTable
CREATE TABLE "User" (
    "id" TEXT NOT NULL,
    "displayName" TEXT NOT NULL,

    CONSTRAINT "User_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "PublicKey" (
    "key" TEXT NOT NULL,
    "userId" TEXT NOT NULL,

    CONSTRAINT "PublicKey_pkey" PRIMARY KEY ("key")
);

-- AddForeignKey
ALTER TABLE "PublicKey" ADD CONSTRAINT "PublicKey_userId_fkey" FOREIGN KEY ("userId") REFERENCES "User"("id") ON DELETE RESTRICT ON UPDATE CASCADE;
