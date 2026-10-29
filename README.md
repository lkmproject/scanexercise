# scanexercise

移除試卷上的手寫、塗鴉與箭頭，並保留印刷內容。擦除使用伺服器上的固定 prompt 呼叫千問圖像編輯（中國香港地域的 `qwen-image-3.0-pro`）。

## 後端

```bash
cd backend
python3 -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
cp .env.example .env
```

在阿里雲百煉的中國香港工作空間建立 API key，把 `DASHSCOPE_API_KEY` 和 `QWEN_WORKSPACE_ID` 填入 `.env`。金鑰、工作空間和模型必須同屬香港地域。然後：

```bash
uvicorn app.main:app --reload --host 0.0.0.0 --port 8000
```

測試：

```bash
cd backend && .venv/bin/pytest
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
