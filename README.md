# scanexercise

移除試卷上的手寫、塗鴉與箭頭，並保留印刷內容。擦除使用 TextIn「自动擦除手写文字」。後端使用 Go 與 [Gin](https://github.com/gin-gonic/gin)。

## 後端

```bash
cd backend
cp .env.example .env
```

到 [TextIn 工作台](https://www.textin.com/) → 帳號設定 → 開發者資訊，把 `TEXTIN_APP_ID`（x-ti-app-id）與 `TEXTIN_SECRET_CODE`（x-ti-secret-code）填入 `.env`，並開通「自动擦除手写文字」。Google 登入另需 `GOOGLE_WEB_CLIENT_ID`、`JWT_SECRET`、`AWS_REGION=ap-southeast-1`，以及可寫入 DynamoDB `Users` / `Jobs` 與 S3 `scanexercise` 的 AWS 憑證。有 AWS 憑證時本機也走 S3 + Jobs 表，不寫本地碟。然後：

```bash
go run ./cmd/server
```

預設聽 `0.0.0.0:8000`。可用 `PORT` 改埠。

所有 API 都在 `/scanexercise` 底下（例如 `POST /scanexercise/auth/google`、`GET /scanexercise/me`）。Job 狀態在 DynamoDB `Jobs`（`pk=PROJECT#scanexercise#JOB#<id>`），檔案在 S3 `scanexercise` 的 `jobs/{id}/...`。建置：

```bash
cd backend
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -o bootstrap ./cmd/lambda
zip function.zip bootstrap
```

環境變數設 `JWT_SECRET`、`GOOGLE_WEB_CLIENT_ID`、`TEXTIN_APP_ID`、`TEXTIN_SECRET_CODE`。Lambda timeout 設 **15 分鐘**（API Gateway 仍是 30 秒；擦除用同一 function 非同步再調一次）。執行角色需：DynamoDB `Users` 與 `Jobs`、S3 `arn:aws:s3:::scanexercise` 與 `arn:aws:s3:::scanexercise/*`、`lambda:InvokeFunction` 對自己。`Jobs` 表主鍵 `pk`+`sk`，TTL 屬性 `ttl`。S3 建議 lifecycle 24 小時刪 `jobs/`。API Gateway HTTP API 用 `ANY /scanexercise/{proxy+}`。App 的 `EXPO_PUBLIC_API_BASE_URL` 設到 `https://<api-id>.execute-api.ap-southeast-1.amazonaws.com/scanexercise`。單張圖超過約 6–10MB 時 Gateway 會擋，這版仍走 multipart。

測試：

```bash
cd backend && go test ./...
```

## App

React Native 0.88（Expo SDK 58）。文件掃描使用 iOS VisionKit 與 Android ML Kit，需要開發建置，不能用 Expo Go。

```bash
cd mobile
npm install
npx expo run:ios
```

模擬器預設連 `http://localhost:8000/scanexercise`。Android 模擬器預設連 `http://10.0.2.2:8000/scanexercise`。實機請設定：

```bash
EXPO_PUBLIC_API_BASE_URL=http://<電腦IP>:8000/scanexercise npx expo run:ios
```

iOS 的 VisionKit 不能從相簿匯入，相簿選圖走系統照片選擇器。Android 在掃描畫面中從相簿匯入。
