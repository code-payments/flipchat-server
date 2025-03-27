-- CreateTable
CREATE TABLE "flipchat_activity_feeds" (
    "id" TEXT NOT NULL,
    "userId" TEXT NOT NULL,
    "activityFeedType" SMALLINT NOT NULL DEFAULT 0,
    "notificationType" INTEGER NOT NULL DEFAULT 0,
    "count" BIGINT NOT NULL DEFAULT 0,
    "chatId" TEXT,
    "messageId" TEXT,
    "ts" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "flipchat_activity_feeds_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE INDEX "flipchat_activity_feeds_userId_activityFeedType_ts_idx" ON "flipchat_activity_feeds"("userId", "activityFeedType", "ts" DESC);
