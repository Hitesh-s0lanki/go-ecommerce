#!/bin/bash
# Creates the uploads bucket once LocalStack is ready. Mounted into
# /etc/localstack/init/ready.d, which LocalStack runs on startup.
set -euo pipefail

BUCKET="${AWS_S3_BUCKET:-ecommerce-uploads}"

# Idempotent: the volume persists across restarts, so this runs again on a
# bucket that already exists.
if awslocal s3api head-bucket --bucket "$BUCKET" 2>/dev/null; then
  echo "bucket $BUCKET already exists"
  exit 0
fi

awslocal s3 mb "s3://$BUCKET"

# Development only. The API stores a public URL for each image and nothing
# signs it, so the bucket has to serve those objects to anonymous readers.
# A real deployment should keep the bucket private and put a CDN in front of
# it (UPLOAD_PUBLIC_BASE_URL) rather than copy this.
awslocal s3api put-bucket-policy --bucket "$BUCKET" --policy "{
  \"Version\": \"2012-10-17\",
  \"Statement\": [{
    \"Sid\": \"PublicReadForDevelopment\",
    \"Effect\": \"Allow\",
    \"Principal\": \"*\",
    \"Action\": \"s3:GetObject\",
    \"Resource\": \"arn:aws:s3:::$BUCKET/*\"
  }]
}"

echo "bucket $BUCKET created"
