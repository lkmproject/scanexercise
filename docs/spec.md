# 試卷手寫筆跡清除工具技術規格

## 1. 概述

本工具讓使用者用手機拍下或選取試卷圖片，自動移除所有非印刷筆跡，並保留印刷內容。擦除由 TextIn「自动擦除手写文字」完成。處理後的頁面可預覽，並下載為 PDF。

允許少量失敗：手寫蓋在印刷圖片上而分不開時，留在圖上；蓋住印刷字而分不開時，保留印刷字，寧可留下少量筆跡。這類頁面仍照常輸出。

## 2. 目標與非目標

### 目標

- 移除手寫字、塗鴉、箭頭、圈選、畫線、螢光筆，以及紅、藍、黑筆和鉛筆。
- 保留印刷題目、選項、頁碼、格線、表格、公式、印刷圖片與線圖。
- 印刷文字保持原圖的繁體或簡體，不轉換、不改字、不新增文字、不重排版面。
- 提供預覽，並可將結果下載為 PDF。
- 支援以手機拍攝或從相簿上傳。

### 非目標

- 不保證像素級還原。
- 不在手寫無法與印刷圖片或印刷字分離時挖空印刷內容。
- 不把「印刷區 OCR 前後不一致」當成整頁作廢條件。
- 不處理多頁合成為一份已裝訂的原始檔以外的版面重編。每一張上傳圖對應 PDF 的一頁；一次上傳多張時，依上傳順序組成一份 PDF。

## 3. 使用流程

1. 使用者在 App 內啟動文件掃描，或從相簿選擇試卷圖片。
2. 客戶端先做紙張偵測、透視校正與裁切，再上傳。
3. 伺服器校正方向、轉成 JPEG，並把長邊縮到模型可接受的大小。
4. 伺服器將圖送交 TextIn 手寫擦除，只採用回傳的圖像。
5. App 顯示處理後的預覽。使用者可下載 PDF。
6. 手寫殘留、圖上筆跡未移除，或少量印刷筆畫變化，仍顯示預覽並允許下載。

處理通常需要數秒到數十秒。上傳後立即返回工作編號，客戶端輪詢狀態，完成後再取預覽與 PDF。

## 4. 擦除服務

擦除由 TextIn `handwritten_erase` 完成，不使用可覆寫的客戶端 prompt。

- 認證：`TEXTIN_APP_ID`、`TEXTIN_SECRET_CODE`（請求頭 `x-ti-app-id`、`x-ti-secret-code`）。
- 請求：JPEG 二進位；`crop` 由工作決定（`0` 關閉切邊，`1` 執行切邊），`dewarp=0`、`binarization=0`、`doc_direction=0`（不彎曲矯正、不增強銳化），`image_type=1` 回彩色圖。
- 回應：只採用 `result.image` 圖像。

API 金鑰只放在伺服器。客戶端不接觸模型。

## 5. 技術堆疊

| 層級 | 選擇 | 職責 |
|---|---|---|
| App | React Native 0.88 | 文件掃描、相簿選圖、上傳、預覽、下載 PDF |
| 掃描 | iOS VisionKit、Android ML Kit Document Scanner | 紙張偵測、透視校正、裁切 |
| 後端 | Go | 收圖、前處理、呼叫模型、工作狀態、輸出 PDF |
| 擦除 | TextIn 自动擦除手写文字 | 去除手寫與批改痕跡，保留印刷內容 |
| PDF | 內嵌 JPEG | 每一頁放一張結果圖，不重新壓縮、不轉成可搜尋文字 |

## 6. 後端行為

### 前處理

- 依 EXIF 轉正。
- 統一編碼為 JPEG。
- 長邊超過上限時等比縮小（預設 2560），再存 JPEG quality 95，以保留印刷字清晰度。
- 單張上傳圖對應一次模型呼叫。

### JSON 回傳

JSON API 一律為 `{ "code": int32, "reason": string, "data": object | null }`，HTTP 狀態固定 `200`。`code = 200` 代表成功，此時 `reason` 固定為 `success`；其它 `code` 代表失敗，`reason` 為英文說明。圖片與 PDF 下載成功時仍回傳檔案本身（HTTP 200）；失敗時改回上述 JSON（仍是 HTTP 200，以 `code` 判斷）。

| code | 名稱 | reason |
|---|---|---|
| 200 | OK | success |
| 1001 | AuthInvalid | invalid google sign-in |
| 1002 | AuthUnavailable | auth service unavailable |
| 1003 | AuthFailed | sign-in failed |
| 1004 | Unauthorized | unauthorized |
| 1101 | InvalidRequest | invalid request |
| 1102 | NoPages | no pages uploaded |
| 1103 | InvalidImage | cannot read image |
| 1104 | TooManyPages | too many pages |
| 1105 | ImageTooLarge | image too large |
| 1201 | JobNotFound | job not found or expired |
| 1202 | JobNotReady | result not ready |
| 1203 | JobFailed | model call failed |
| 1204 | PageNotFound | page not found |
| 1301 | PDFFailed | cannot create pdf |
| 1401 | InsufficientCredits | not enough erases |
| 1901 | Internal | internal error |

### 工作

| 方法 | 路徑 | 行為 |
|---|---|---|
| `POST` | `/scanexercise/auth/google` | 以 Google `id_token` 登入或自動註冊，回傳帳號、剩餘次數 `remaining` 與 3 日 JWT `token`。第一次註冊送 2 次 |
| `GET` | `/scanexercise/me` | 回傳目前帳號與剩餘清除次數 |
| `POST` | `/scanexercise/jobs` | 以 `multipart/form-data` 上傳一張或多張頁面圖；可選欄位 `crop=0|1`（預設 `0`），建立工作並回傳工作編號與扣次後的 `remaining`。按頁預扣次數，次數不足回 `1401`；工作失敗退回 |
| `GET` | `/scanexercise/jobs/{id}` | 回傳狀態：`pending`、`running`、`succeeded`、`failed`，以及目前 `remaining` |
| `GET` | `/scanexercise/jobs/{id}/preview` | 成功後回傳各頁結果圖 |
| `GET` | `/scanexercise/jobs/{id}/pdf` | 成功後回傳 PDF |
| `POST` | `/scanexercise/pdf` | 以目前頁面圖直接產生 PDF |

`/scanexercise/me`、`/scanexercise/jobs` 與 `/scanexercise/pdf` 需 `Authorization: Bearer <token>`。狀態為 `failed` 時回傳可顯示的錯誤，例如模型拒絕、逾時或輸出中沒有圖像。不把「仍有手寫殘留」標成失敗。`GET /jobs/{id}` 成功取得失敗工作時，外層 `code` 仍是 200，失敗寫在 `data.status` / `data.error`。清除按頁計費，餘額存在 DynamoDB `Users` 的 `USER#<id>` / `BALANCE`；`POST /pdf` 不扣次。

### PDF

- 一頁結果圖成為 PDF 的一頁。
- 頁面順序與上傳順序相同。
- 不重新排版題目，不把圖轉成可搜尋文字。

### 檔案生命週期

試卷可能含姓名、學號與分數。原圖、結果圖與 PDF 存在 S3 `scanexercise`（`jobs/{id}/...`），工作 meta 存在 DynamoDB `Jobs`（`pk=PROJECT#scanexercise#JOB#{id}`，`sk=META`）。完成並被下載，或超過保留時間後刪除。保留時間的預設值為 24 小時（DynamoDB TTL 與 S3 lifecycle）。伺服器記錄不寫入試卷影像內容。

## 7. App 行為

- 掃描與相簿匯入都走系統文件掃描，不另做一套裁切介面。
- 上傳中顯示處理狀態，完成後顯示每一頁的預覽。
- 點預覽可進全螢幕，並可返回列表。
- 預覽不阻擋下載。使用者可在仍有殘留筆跡時下載 PDF。
- PDF 經系統分享表輸出。
- 無網路或工作失敗時顯示錯誤，並保留使用者手上的原圖。

## 8. 驗收

上線前用真卷各準備繁體與簡體樣本，且樣本須包含：

- 寫在空白與格線上的手寫字、塗鴉、箭頭與圈選
- 紅筆、藍筆與鉛筆
- 印刷圖片，以及寫在圖片上的筆跡
- 寫在印刷題目上的筆跡

通過條件：

- 空白與格線上的非印刷筆跡大多數已移除。
- 印刷圖片仍在；圖上分不開的筆跡可以留下。
- 題目沒有整段消失、明顯改寫或繁簡轉換。少量筆畫誤差可接受。
- 每一張成功的模型回應都能預覽並下載 PDF。
- 模型失敗時工作為 `failed`，且不會把原圖當成清除結果。

## 9. 已知限制

- 服務在中國大陸公有雲處理影像。
- 極端重疊、極淡鉛筆或複雜塗改仍可能殘留。
- 單頁按次計費。一次工作的頁數應設上限，預設 20 頁。
