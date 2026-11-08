# scanexercise

移除試卷上的手寫、塗鴉與箭頭，並保留印刷內容。擦除使用 TextIn「自动擦除手写文字」。

## 後端

```bash
cd backend
cp .env.example .env
```

到 [TextIn 工作台](https://www.textin.com/) → 帳號設定 → 開發者資訊，把 `TEXTIN_APP_ID`（x-ti-app-id）與 `TEXTIN_SECRET_CODE`（x-ti-secret-code）填入 `.env`，並開通「自动擦除手写文字」。然後：

```bash
go run ./cmd/server
```

預設聽 `0.0.0.0:8000`。可用 `PORT` 改埠。

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

模擬器預設連 `http://localhost:8000`。Android 模擬器預設連 `http://10.0.2.2:8000`。實機請設定：

```bash
EXPO_PUBLIC_API_BASE_URL=http://<電腦IP>:8000 npx expo run:ios
```

iOS 的 VisionKit 不能從相簿匯入，相簿選圖走系統照片選擇器。Android 在掃描畫面中從相簿匯入。
