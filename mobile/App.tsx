import { useEffect, useState } from 'react';
import {
  ActivityIndicator,
  Image,
  Platform,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  View,
} from 'react-native';
import { StatusBar } from 'expo-status-bar';
import { scanDocument } from 'expo-document-scanner';
import { cacheDirectory, downloadAsync } from 'expo-file-system/legacy';
import * as ImagePicker from 'expo-image-picker';
import * as Sharing from 'expo-sharing';

import { createJob, getJob, type Job } from './src/api';
import { API_BASE_URL, MAX_PAGES } from './src/config';

type Phase = 'editing' | 'processing' | 'done';

export default function App() {
  const [pages, setPages] = useState<string[]>([]);
  const [phase, setPhase] = useState<Phase>('editing');
  const [job, setJob] = useState<Job | null>(null);
  const [previews, setPreviews] = useState<string[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [sharing, setSharing] = useState(false);

  useEffect(() => {
    if (phase !== 'processing' || job == null) {
      return;
    }
    let cancelled = false;
    let settled = false;
    let failures = 0;

    const tick = () => {
      void getJob(job.id)
        .then(async (next) => {
          if (cancelled || settled) {
            return;
          }
          failures = 0;
          if (next.status === 'succeeded') {
            settled = true;
            try {
              const uris = await loadPreviews(next);
              if (!cancelled) {
                setPreviews(uris);
                setPhase('done');
              }
            } catch {
              if (!cancelled) {
                setError('無法取得預覽');
                setPhase('editing');
              }
            }
            return;
          }
          if (next.status === 'failed') {
            settled = true;
            setError(next.error ?? '模型呼叫失敗');
            setPhase('editing');
          }
        })
        .catch(() => {
          if (cancelled || settled) {
            return;
          }
          failures += 1;
          if (failures >= 3) {
            settled = true;
            setError('無法連線到伺服器');
            setPhase('editing');
          }
        });
    };

    tick();
    const timer = setInterval(tick, 1500);
    return () => {
      cancelled = true;
      clearInterval(timer);
    };
  }, [phase, job]);

  function addPages(uris: string[]) {
    setError(null);
    setPages((current) => [...current, ...uris].slice(0, MAX_PAGES));
  }

  async function scan() {
    if (pages.length >= MAX_PAGES) {
      setError(`一次最多 ${MAX_PAGES} 頁`);
      return;
    }
    try {
      const result = await scanDocument({
        quality: 0.92,
        maxNumDocuments: MAX_PAGES - pages.length,
        galleryImportAllowed: true,
      });
      addPages(result.pages.map((page) => page.uri));
    } catch (caught) {
      if (!isCancel(caught)) {
        setError(caught instanceof Error ? caught.message : '無法開啟掃描');
      }
    }
  }

  async function pickFromLibrary() {
    if (pages.length >= MAX_PAGES) {
      setError(`一次最多 ${MAX_PAGES} 頁`);
      return;
    }
    const result = await ImagePicker.launchImageLibraryAsync({
      mediaTypes: ['images'],
      allowsMultipleSelection: true,
      selectionLimit: MAX_PAGES - pages.length,
      quality: 0.92,
    });
    if (!result.canceled) {
      addPages(result.assets.map((asset) => asset.uri));
    }
  }

  async function submit() {
    setError(null);
    setPreviews([]);
    try {
      const created = await createJob(pages);
      setJob({
        id: created.id,
        status: created.status,
        page_count: pages.length,
        error: null,
      });
      setPhase('processing');
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '無法連線到伺服器');
      setPhase('editing');
    }
  }

  async function sharePdf() {
    if (job == null) {
      return;
    }
    setSharing(true);
    setError(null);
    try {
      const directory = cacheDirectory;
      if (directory == null) {
        setError('無法儲存檔案');
        return;
      }
      const destination = `${directory}exam-${job.id}.pdf`;
      const downloaded = await downloadAsync(`${API_BASE_URL}/jobs/${job.id}/pdf`, destination);
      if (downloaded.status !== 200) {
        setError('無法下載 PDF');
        return;
      }
      if (!(await Sharing.isAvailableAsync())) {
        setError('這台裝置無法分享檔案');
        return;
      }
      await Sharing.shareAsync(downloaded.uri, {
        mimeType: 'application/pdf',
        UTI: 'com.adobe.pdf',
      });
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : '無法下載 PDF');
    } finally {
      setSharing(false);
    }
  }

  function reset() {
    setPages([]);
    setJob(null);
    setPreviews([]);
    setError(null);
    setPhase('editing');
  }

  const busy = phase === 'processing';

  return (
    <View style={styles.screen}>
      <StatusBar style="dark" />
      <Text style={styles.title}>試卷筆跡清除</Text>
      <Text style={styles.subtitle}>
        移除手寫、塗鴉與箭頭，印刷內容留在頁面上。圖上分不開的筆跡會保留。
      </Text>

      {phase !== 'done' ? (
        <View style={styles.actions}>
          <Pressable style={styles.primary} disabled={busy} onPress={() => void scan()}>
            <Text style={styles.primaryLabel}>掃描試卷</Text>
          </Pressable>
          {Platform.OS === 'ios' ? (
            <Pressable style={styles.secondary} disabled={busy} onPress={() => void pickFromLibrary()}>
              <Text style={styles.secondaryLabel}>從相簿選擇</Text>
            </Pressable>
          ) : null}
        </View>
      ) : null}

      {Platform.OS === 'android' && phase !== 'done' ? (
        <Text style={styles.hint}>Android 的掃描畫面裡可以改從相簿匯入。</Text>
      ) : null}

      <ScrollView contentContainerStyle={styles.list}>
        {(phase === 'done' ? previews : pages).map((uri, index) => (
          <View key={`${uri}-${index}`} style={styles.card}>
            <Image source={{ uri }} style={styles.image} resizeMode="contain" />
            <View style={styles.cardRow}>
              <Text style={styles.cardLabel}>第 {index + 1} 頁</Text>
              {phase === 'editing' ? (
                <Pressable onPress={() => setPages((current) => current.filter((_, item) => item !== index))}>
                  <Text style={styles.remove}>移除</Text>
                </Pressable>
              ) : null}
            </View>
          </View>
        ))}
      </ScrollView>

      {error ? <Text style={styles.error}>{error}</Text> : null}

      {phase === 'processing' ? (
        <View style={styles.statusRow}>
          <ActivityIndicator color="#9f2d2d" />
          <Text style={styles.status}>正在清除筆跡</Text>
        </View>
      ) : null}

      {phase === 'editing' && pages.length > 0 ? (
        <Pressable style={styles.primary} onPress={() => void submit()}>
          <Text style={styles.primaryLabel}>開始清除</Text>
        </Pressable>
      ) : null}

      {phase === 'done' ? (
        <View style={styles.actions}>
          <Pressable style={styles.primary} disabled={sharing} onPress={() => void sharePdf()}>
            <Text style={styles.primaryLabel}>{sharing ? '準備 PDF' : '下載 PDF'}</Text>
          </Pressable>
          <Pressable style={styles.secondary} onPress={reset}>
            <Text style={styles.secondaryLabel}>清除並重來</Text>
          </Pressable>
        </View>
      ) : null}
    </View>
  );
}

async function loadPreviews(job: Job): Promise<string[]> {
  const paths = job.previews ?? [];
  if (cacheDirectory == null) {
    throw new Error('無法儲存檔案');
  }
  const uris: string[] = [];
  for (const [index, path] of paths.entries()) {
    const destination = `${cacheDirectory}exam-${job.id}-${index}.jpg`;
    const downloaded = await downloadAsync(`${API_BASE_URL}${path}`, destination);
    if (downloaded.status !== 200) {
      throw new Error('無法取得預覽');
    }
    uris.push(downloaded.uri);
  }
  return uris;
}

function isCancel(error: unknown): boolean {
  const message = error instanceof Error ? error.message : String(error);
  return /cancel/i.test(message);
}

const styles = StyleSheet.create({
  screen: {
    flex: 1,
    backgroundColor: '#f4f1ea',
    paddingTop: 64,
    paddingHorizontal: 20,
    paddingBottom: 28,
  },
  title: {
    fontSize: 28,
    fontWeight: '700',
    color: '#1c1917',
  },
  subtitle: {
    marginTop: 8,
    fontSize: 15,
    lineHeight: 22,
    color: '#57534e',
  },
  actions: {
    marginTop: 20,
    gap: 10,
  },
  primary: {
    backgroundColor: '#9f2d2d',
    borderRadius: 12,
    paddingVertical: 14,
    alignItems: 'center',
  },
  primaryLabel: {
    color: '#fffaf5',
    fontSize: 16,
    fontWeight: '700',
  },
  secondary: {
    borderWidth: 1,
    borderColor: '#d6d3d1',
    borderRadius: 12,
    paddingVertical: 14,
    alignItems: 'center',
    backgroundColor: '#fffaf5',
  },
  secondaryLabel: {
    color: '#1c1917',
    fontSize: 16,
    fontWeight: '600',
  },
  hint: {
    marginTop: 10,
    color: '#78716c',
    fontSize: 13,
  },
  list: {
    paddingVertical: 16,
    gap: 12,
  },
  card: {
    backgroundColor: '#fffaf5',
    borderRadius: 12,
    padding: 10,
  },
  image: {
    width: '100%',
    height: 280,
    backgroundColor: '#e7e5e4',
  },
  cardRow: {
    marginTop: 8,
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
  },
  cardLabel: {
    color: '#44403c',
    fontSize: 14,
  },
  remove: {
    color: '#9f2d2d',
    fontSize: 14,
  },
  error: {
    color: '#9f2d2d',
    marginBottom: 8,
  },
  statusRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8,
    marginBottom: 12,
  },
  status: {
    color: '#44403c',
  },
});
