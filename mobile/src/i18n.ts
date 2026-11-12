import { NativeModules } from 'react-native';

import { MAX_PAGES } from './config';
import {
  documentDirectory,
  readAsStringAsync,
  writeAsStringAsync,
} from 'expo-file-system/legacy';

export type Locale = 'zh-Hant' | 'zh-Hans' | 'en';

export const LOCALES: { id: Locale; label: string }[] = [
  { id: 'zh-Hant', label: '繁' },
  { id: 'zh-Hans', label: '简' },
  { id: 'en', label: 'EN' },
];

type Copy = {
  subtitle: string;
  meTitle: string;
  meCopy: string;
  backHome: string;
  bannerTitle: string;
  bannerHint: string;
  learnMore: string;
  aboutTitle: string;
  aboutClose: string;
  aboutFeatures: (n: number) => string[];
  album: string;
  maxPages: (n: number) => string;
  albumAndroid: string;
  erase: string;
  keepPrint: string;
  eraseNew: string;
  pending: string;
  noNewPages: string;
  crop: string;
  cropHelpTitle: string;
  cropHelp: string;
  binarization: string;
  binarizationHelpTitle: string;
  binarizationHelp: string;
  empty: string;
  totalSize: (kb: number) => string;
  clearing: string;
  clearingTimeout: string;
  jobId: (id: string) => string;
  preparingPdf: string;
  downloadPdf: string;
  shareBlank: string;
  erased: (n: number) => string;
  erasedHint: string;
  pendingLeft: (n: number) => string;
  addMore: string;
  again: string;
  againConfirm: string;
  againConfirmCopy: string;
  cancel: string;
  confirm: string;
  home: string;
  me: string;
  back: string;
  page: (n: number) => string;
  pageCount: (n: number, total: number) => string;
  seeBefore: string;
  seeAfter: string;
  before: string;
  after: string;
  previewFailed: string;
  modelFailed: string;
  serverFailed: string;
  tooManyPages: (n: number) => string;
  scanFailed: string;
  cameraDenied: string;
  noPages: string;
  saveFailed: string;
  pdfFailed: string;
  shareUnavailable: string;
  appleLogin: string;
  googleLogin: string;
  close: string;
  googleFailed: string;
  signedInBadge: string;
  remainingErase: string;
  remainingTimes: string;
  notEnoughErases: string;
  logout: string;
  authInvalid: string;
  authUnavailable: string;
  authFailed: string;
  authUnauthorized: string;
  invalidRequest: string;
  noPagesUploaded: string;
  invalidImage: string;
  imageTooLarge: string;
  jobNotFound: string;
  jobNotReady: string;
  pageNotFound: string;
  internalError: string;
};

export const copy: Record<Locale, Copy> = {
  'zh-Hant': {
    subtitle: '拍完就能重做。',
    meTitle: '我的',
    meCopy: '試卷暫時只留在這次處理裡，下載 PDF 後就會刪除。',
    backHome: '回首頁',
    bannerTitle: '開學特輯',
    bannerHint: '拍試卷清筆跡，空白卷隨時練',
    learnMore: '了解更多',
    aboutTitle: '空白卷能做什麼',
    aboutClose: '知道了',
    aboutFeatures: (n) => [
      `拍照或從相簿匯入試卷，一次最多 ${n} 頁。`,
      '清除手寫、塗鴉和批改痕跡，印刷題目原樣留下。',
      '繁體簡體不會互轉，只改 App 介面語言。',
      '需要時可開啟切邊，裁掉紙張外的邊。',
      '需要時可開啟二值化，讓卷面更清楚。',
      '處理後可放大查看，左右滑動換頁。',
      '按「看修改前」對比原圖與清除結果。',
      '下載 PDF 帶走；試卷只留在這次處理裡。',
    ],
    album: '相簿',
    maxPages: (n) => `最多 ${n} 頁`,
    albumAndroid: '掃描畫面裡可匯入',
    erase: '清除',
    keepPrint: '印刷留下',
    eraseNew: '只清新圖',
    pending: '未清除',
    noNewPages: '沒有新圖要清除。',
    crop: '切邊',
    cropHelpTitle: '什麼是切邊',
    cropHelp: '打開後會裁掉紙張外面的桌面、背景，只留下試卷本身。拍歪一點的邊也比較容易修齊。',
    binarization: '二值化',
    binarizationHelpTitle: '什麼是二值化',
    binarizationHelp:
      '打開後會把字和線條變清楚一點，紙上的髒點、陰影也比較不明顯。還是彩色，不會變成只有黑和白的影印效果。',
    empty: '還沒有頁面。點下面相機掃描。',
    totalSize: (kb) => `共 ${kb} KB`,
    clearing: '正在清除筆跡',
    clearingTimeout: '清除逾時，請再試一次',
    jobId: (id) => `工作 ${id}`,
    preparingPdf: '準備 PDF',
    downloadPdf: '下載 PDF',
    shareBlank: '分享空白卷',
    erased: (n) => `已清除 ${n} 頁`,
    erasedHint: '點圖可對比修改前後',
    pendingLeft: (n) => `還有 ${n} 頁未清`,
    addMore: '再加頁面',
    again: '重來',
    againConfirm: '要重來嗎？',
    againConfirmCopy: '已匯入和清除的頁面都會刪掉。',
    cancel: '取消',
    confirm: '確認',
    home: '首頁',
    me: '我的',
    back: '返回',
    page: (n) => `第 ${n} 頁`,
    pageCount: (n, total) => `${n} / ${total}`,
    seeBefore: '看修改前',
    seeAfter: '看修改後',
    before: '修改前',
    after: '修改後',
    previewFailed: '無法取得預覽',
    modelFailed: '模型呼叫失敗',
    serverFailed: '無法連線到伺服器',
    tooManyPages: (n) => `一次最多 ${n} 頁`,
    scanFailed: '無法開啟相機',
    cameraDenied: '請允許使用相機',
    noPages: '還沒有頁面。先掃，再清。',
    saveFailed: '無法儲存檔案',
    pdfFailed: '無法下載 PDF',
    shareUnavailable: '這台裝置無法分享檔案',
    appleLogin: '透過 Apple 登入',
    googleLogin: '透過 Google 登入',
    close: '關閉',
    googleFailed: '無法使用 Google 登入',
    signedInBadge: '已登入',
    remainingErase: '剩餘清除',
    remainingTimes: '次',
    notEnoughErases: '清除次數不足',
    logout: '登出',
    authInvalid: 'Google 登入無效',
    authUnavailable: '登入服務暫時無法使用',
    authFailed: '登入失敗',
    authUnauthorized: '請重新登入',
    invalidRequest: '請求不正確',
    noPagesUploaded: '沒有上傳頁面',
    invalidImage: '無法讀取圖片',
    imageTooLarge: '圖片太大',
    jobNotFound: '工作不存在或已過期',
    jobNotReady: '結果還沒準備好',
    pageNotFound: '找不到這一頁',
    internalError: '伺服器發生問題',
  },
  'zh-Hans': {
    subtitle: '拍完就能重做。',
    meTitle: '我的',
    meCopy: '试卷暂时只留在这次处理里，下载 PDF 后就会删除。',
    backHome: '回首页',
    bannerTitle: '开学特辑',
    bannerHint: '拍试卷清笔迹，空白卷随时练',
    learnMore: '了解更多',
    aboutTitle: '空白卷能做什么',
    aboutClose: '知道了',
    aboutFeatures: (n) => [
      `拍照或从相册导入试卷，一次最多 ${n} 页。`,
      '清除手写、涂鸦和批改痕迹，印刷题目原样留下。',
      '繁体简体不会互转，只改 App 界面语言。',
      '需要时可开启切边，裁掉纸张外的边。',
      '需要时可开启二值化，让卷面更清楚。',
      '处理后可放大查看，左右滑动换页。',
      '按「看修改前」对比原图与清除结果。',
      '下载 PDF 带走；试卷只留在这次处理里。',
    ],
    album: '相册',
    maxPages: (n) => `最多 ${n} 页`,
    albumAndroid: '扫描画面里可导入',
    erase: '清除',
    keepPrint: '印刷留下',
    eraseNew: '只清新图',
    pending: '未清除',
    noNewPages: '没有新图要清除。',
    crop: '切边',
    cropHelpTitle: '什么是切边',
    cropHelp: '打开后会裁掉纸张外面的桌面、背景，只留下试卷本身。拍歪一点的边也比较容易修齐。',
    binarization: '二值化',
    binarizationHelpTitle: '什么是二值化',
    binarizationHelp:
      '打开后会把字和线条变清楚一点，纸上的脏点、阴影也比较不明显。还是彩色，不会变成只有黑和白的影印效果。',
    empty: '还没有页面。点下面相机扫描。',
    totalSize: (kb) => `共 ${kb} KB`,
    clearing: '正在清除笔迹',
    clearingTimeout: '清除超时，请再试一次',
    jobId: (id) => `工作 ${id}`,
    preparingPdf: '准备 PDF',
    downloadPdf: '下载 PDF',
    shareBlank: '分享空白卷',
    erased: (n) => `已清除 ${n} 页`,
    erasedHint: '点图可对比修改前后',
    pendingLeft: (n) => `还有 ${n} 页未清`,
    addMore: '再加页面',
    again: '重来',
    againConfirm: '要重来吗？',
    againConfirmCopy: '已导入和清除的页面都会删掉。',
    cancel: '取消',
    confirm: '确认',
    home: '首页',
    me: '我的',
    back: '返回',
    page: (n) => `第 ${n} 页`,
    pageCount: (n, total) => `${n} / ${total}`,
    seeBefore: '看修改前',
    seeAfter: '看修改后',
    before: '修改前',
    after: '修改后',
    previewFailed: '无法取得预览',
    modelFailed: '模型调用失败',
    serverFailed: '无法连到服务器',
    tooManyPages: (n) => `一次最多 ${n} 页`,
    scanFailed: '无法开启相机',
    cameraDenied: '请允许使用相机',
    noPages: '还没有页面。先扫，再清。',
    saveFailed: '无法保存文件',
    pdfFailed: '无法下载 PDF',
    shareUnavailable: '这台设备无法分享文件',
    appleLogin: '通过 Apple 登录',
    googleLogin: '通过 Google 登录',
    close: '关闭',
    googleFailed: '无法使用 Google 登录',
    signedInBadge: '已登录',
    remainingErase: '剩余清除',
    remainingTimes: '次',
    notEnoughErases: '清除次数不足',
    logout: '登出',
    authInvalid: 'Google 登录无效',
    authUnavailable: '登录服务暂时无法使用',
    authFailed: '登录失败',
    authUnauthorized: '请重新登录',
    invalidRequest: '请求不正确',
    noPagesUploaded: '没有上传页面',
    invalidImage: '无法读取图片',
    imageTooLarge: '图片太大',
    jobNotFound: '工作不存在或已过期',
    jobNotReady: '结果还没准备好',
    pageNotFound: '找不到这一页',
    internalError: '服务器发生问题',
  },
  en: {
    subtitle: 'Snap it, then start over.',
    meTitle: 'Me',
    meCopy: 'Papers stay only for this session. They are deleted after you download the PDF.',
    backHome: 'Home',
    bannerTitle: 'Back to school',
    bannerHint: 'Clear handwriting. Practice on a blank sheet.',
    learnMore: 'Learn more',
    aboutTitle: 'What Blank Sheet does',
    aboutClose: 'Got it',
    aboutFeatures: (n) => [
      `Scan or import exam pages, up to ${n} at a time.`,
      'Erase handwriting and marks. Printed questions stay.',
      'Traditional and simplified text are never converted.',
      'Turn on crop when you want the paper edges trimmed.',
      'Turn on binarize to sharpen the page.',
      'Pinch to zoom, swipe to the next page.',
      'Tap Before to compare the original with the result.',
      'Download a PDF. Pages stay only for this session.',
    ],
    album: 'Album',
    maxPages: (n) => `Up to ${n} pages`,
    albumAndroid: 'Import from the scanner',
    erase: 'Erase',
    keepPrint: 'Print stays',
    eraseNew: 'New pages only',
    pending: 'New',
    noNewPages: 'No new pages to erase.',
    crop: 'Crop',
    cropHelpTitle: 'What crop does',
    cropHelp: 'Trims the table or background around the paper, so only the exam sheet remains. Slightly crooked edges are also straightened.',
    binarization: 'Binarize',
    binarizationHelpTitle: 'What binarize does',
    binarizationHelp:
      'Turns on a sharpening filter so text and lines look clearer, and stains or shadows fade a bit. The page stays in color; it does not become a black-and-white photocopy.',
    empty: 'No pages yet. Tap the camera below.',
    totalSize: (kb) => `Total ${kb} KB`,
    clearing: 'Erasing handwriting',
    clearingTimeout: 'Erasing timed out. Try again.',
    jobId: (id) => `Job ${id}`,
    preparingPdf: 'Preparing PDF',
    downloadPdf: 'Download PDF',
    shareBlank: 'Share the blank paper',
    erased: (n) => (n === 1 ? '1 page erased' : `${n} pages erased`),
    erasedHint: 'Tap a page to compare before and after',
    pendingLeft: (n) => (n === 1 ? '1 new page left' : `${n} new pages left`),
    addMore: 'Add pages',
    again: 'Start over',
    againConfirm: 'Start over?',
    againConfirmCopy: 'Imported and erased pages will be removed.',
    cancel: 'Cancel',
    confirm: 'Confirm',
    home: 'Home',
    me: 'Me',
    back: 'Back',
    page: (n) => `Page ${n}`,
    pageCount: (n, total) => `${n} / ${total}`,
    seeBefore: 'Before',
    seeAfter: 'After',
    before: 'Before',
    after: 'After',
    previewFailed: "Couldn't load preview",
    modelFailed: 'Model request failed',
    serverFailed: "Couldn't reach the server",
    tooManyPages: (n) => `Up to ${n} pages at a time`,
    scanFailed: "Couldn't open the camera",
    cameraDenied: 'Allow camera access to take a photo',
    noPages: 'No pages yet. Scan, then erase.',
    saveFailed: "Couldn't save the file",
    pdfFailed: "Couldn't download the PDF",
    shareUnavailable: "This device can't share files",
    appleLogin: 'Continue with Apple',
    googleLogin: 'Continue with Google',
    close: 'Close',
    googleFailed: "Couldn't sign in with Google",
    signedInBadge: 'Signed in',
    remainingErase: 'Erases left',
    remainingTimes: '',
    notEnoughErases: 'Not enough erases',
    logout: 'Log out',
    authInvalid: 'Invalid Google sign-in',
    authUnavailable: 'Sign-in is unavailable',
    authFailed: 'Sign-in failed',
    authUnauthorized: 'Please sign in again',
    invalidRequest: 'Invalid request',
    noPagesUploaded: 'No pages uploaded',
    invalidImage: "Can't read the image",
    imageTooLarge: 'Image is too large',
    jobNotFound: 'Job not found or expired',
    jobNotReady: "Result isn't ready",
    pageNotFound: 'Page not found',
    internalError: 'Something went wrong',
  },
};

const apiReasons: Record<string, keyof Copy> = {
  'invalid google sign-in': 'authInvalid',
  'auth service unavailable': 'authUnavailable',
  'sign-in failed': 'authFailed',
  'unauthorized': 'authUnauthorized',
  'invalid request': 'invalidRequest',
  'no pages uploaded': 'noPagesUploaded',
  'cannot read image': 'invalidImage',
  'too many pages': 'tooManyPages',
  'image too large': 'imageTooLarge',
  'job not found or expired': 'jobNotFound',
  'result not ready': 'jobNotReady',
  'model call failed': 'modelFailed',
  'page not found': 'pageNotFound',
  'cannot create pdf': 'pdfFailed',
  'not enough erases': 'notEnoughErases',
  'internal error': 'internalError',
  'model timed out': 'clearingTimeout',
};

export function localizeApiMessage(t: Copy, message: string): string {
  const reason = message.includes(': ') ? message.slice(message.lastIndexOf(': ') + 2) : message;
  const key = apiReasons[reason];
  if (key == null) {
    return message;
  }
  const value = t[key];
  if (typeof value === 'function') {
    return t.tooManyPages(MAX_PAGES);
  }
  return value;
}

const localeFile = documentDirectory == null ? null : `${documentDirectory}locale.txt`;

function isLocale(value: string): value is Locale {
  return value === 'zh-Hant' || value === 'zh-Hans' || value === 'en';
}

function localeFromTag(tag: string): Locale | null {
  const normalized = tag.replace(/_/g, '-').toLowerCase();
  if (
    normalized.includes('hant') ||
    normalized.startsWith('zh-tw') ||
    normalized.startsWith('zh-hk') ||
    normalized.startsWith('zh-mo')
  ) {
    return 'zh-Hant';
  }
  if (
    normalized.includes('hans') ||
    normalized.startsWith('zh-cn') ||
    normalized.startsWith('zh-sg')
  ) {
    return 'zh-Hans';
  }
  if (normalized === 'zh' || normalized.startsWith('zh-')) {
    return 'zh-Hant';
  }
  if (normalized === 'en' || normalized.startsWith('en-')) {
    return 'en';
  }
  return null;
}

function deviceLanguageTags(): string[] {
  const tags: string[] = [];
  const push = (value: unknown) => {
    if (typeof value === 'string' && value.length > 0) {
      if (value.includes(',')) {
        value.split(',').forEach((part) => push(part.trim()));
        return;
      }
      tags.push(value);
    } else if (Array.isArray(value)) {
      value.forEach(push);
    }
  };

  try {
    const identifier = NativeModules.I18nManager?.localeIdentifier;
    if (typeof identifier === 'string') {
      push(identifier);
    }
  } catch {
    // New Architecture may not expose this field.
  }

  try {
    const nav = typeof navigator === 'undefined' ? undefined : navigator;
    push(nav?.languages);
    push(nav?.language);
  } catch {
    // Ignore missing browser locale.
  }

  try {
    push(Intl.DateTimeFormat().resolvedOptions().locale);
  } catch {
    // Ignore missing Intl locale.
  }

  return tags;
}

export function localeFromDevice(): Locale {
  try {
    for (const tag of deviceLanguageTags()) {
      const matched = localeFromTag(tag);
      if (matched != null) {
        return matched;
      }
    }
  } catch {
    // Fall through to English.
  }
  return 'en';
}

export async function loadLocale(): Promise<Locale> {
  if (localeFile != null) {
    try {
      const raw = await readAsStringAsync(localeFile);
      if (isLocale(raw)) {
        return raw;
      }
    } catch {
      // First launch has no saved locale yet.
    }
  }
  return localeFromDevice();
}

export async function saveLocale(locale: Locale): Promise<void> {
  if (localeFile == null) {
    return;
  }
  try {
    await writeAsStringAsync(localeFile, locale);
  } catch {
    // Keep the in-memory locale even if disk write fails.
  }
}
