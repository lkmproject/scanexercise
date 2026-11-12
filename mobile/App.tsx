import { useEffect, useRef, useState } from 'react';
import {
  ActivityIndicator,
  FlatList,
  Image,
  Modal,
  PanResponder,
  Platform,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  View,
  useWindowDimensions,
  type GestureResponderEvent,
  type ViewToken,
} from 'react-native';
import { StatusBar } from 'expo-status-bar';
import { cacheDirectory, downloadAsync, readAsStringAsync } from 'expo-file-system/legacy';
import * as ImagePicker from 'expo-image-picker';
import * as Sharing from 'expo-sharing';

import { createJob, failFromEnvelope, getJob, getMe, isUnauthorized, type Job } from './src/api';
import AuthModal from './src/AuthModal';
import { API_BASE_URL, MAX_PAGES } from './src/config';
import { uriBytes } from './src/fileBytes';
import { writeExamPdf } from './src/pdf';
import { fitForUpload } from './src/prepareImage';
import { authHeaders, clearAccessToken, loadSession, notifyUnauthorized, saveCurrentSession, subscribeUnauthorized } from './src/session';
import { copy, loadLocale, localeFromDevice, localizeApiMessage, LOCALES, saveLocale, type Locale } from './src/i18n';

type Phase = 'editing' | 'processing' | 'done';
type Tab = 'home' | 'me';
type AuthProvider = 'google' | 'apple';
type Sheet = {
  id: string;
  original: string;
  processed: string | null;
  bytes: number;
};

type DragState = {
  id: string;
  from: number;
  x: number;
  y: number;
  uri: string;
};

const FAKE_ACCOUNT = {
  name: 'Kwongming',
  email: 'kwongming@gmail.com',
  remaining: 0,
  provider: null as AuthProvider | null,
};

let sheetSeq = 0;
function nextSheetId(): string {
  sheetSeq += 1;
  return `s${sheetSeq}`;
}

function GoogleMark() {
  return (
    <View style={styles.googleGWrap} accessibilityElementsHidden>
      <View style={styles.googleGRing} />
      <View style={styles.googleGGap} />
      <View style={styles.googleGBar} />
    </View>
  );
}

function RefreshMark() {
  return (
    <View style={styles.meRefresh} accessibilityElementsHidden>
      <View style={styles.meRefreshArc} />
      <View style={styles.meRefreshArrow} />
    </View>
  );
}

function moveItem<T>(list: T[], from: number, to: number): T[] {
  if (from === to || from < 0 || to < 0 || from >= list.length || to >= list.length) {
    return list;
  }
  const next = list.slice();
  const [item] = next.splice(from, 1);
  next.splice(to, 0, item);
  return next;
}

function FlagSwitch({
  label,
  value,
  disabled,
  onToggle,
  onHelp,
  helpLabel,
  hintDark,
  rowStyle,
  labelStyle,
}: {
  label: string;
  value: boolean;
  disabled?: boolean;
  onToggle: () => void;
  onHelp?: () => void;
  helpLabel?: string;
  hintDark?: boolean;
  rowStyle: object;
  labelStyle: object;
}) {
  return (
    <View style={rowStyle}>
      <View style={styles.flagLabelRow}>
        {onHelp ? (
          <Pressable
            onPress={onHelp}
            hitSlop={6}
            accessibilityRole="button"
            accessibilityLabel={helpLabel ?? label}
          >
            <View style={[styles.helpDot, hintDark ? styles.helpDotDark : null]}>
              <Text style={[styles.helpMark, hintDark ? styles.helpMarkDark : null]}>?</Text>
            </View>
          </Pressable>
        ) : null}
        <Text style={labelStyle}>{label}</Text>
      </View>
      <Pressable
        disabled={disabled}
        onPress={onToggle}
        hitSlop={8}
        accessibilityRole="switch"
        accessibilityState={{ checked: value }}
        accessibilityLabel={label}
        style={styles.flagSwitch}
      >
        <View style={[styles.switchTrack, value ? styles.switchOn : null]}>
          <View style={[styles.switchThumb, value ? styles.switchThumbOn : null]} />
        </View>
      </Pressable>
    </View>
  );
}

export default function App() {
  const [sheets, setSheets] = useState<Sheet[]>([]);
  const [phase, setPhase] = useState<Phase>('editing');
  const [job, setJob] = useState<Job | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [sharing, setSharing] = useState(false);
  const [selectedPage, setSelectedPage] = useState<{ uri: string; index: number } | null>(null);
  const [tab, setTab] = useState<Tab>('home');
  const [crop, setCrop] = useState(true);
  const [binarization, setBinarization] = useState(true);
  const [optionHelp, setOptionHelp] = useState<{ title: string; copy: string } | null>(null);
  const [aboutOpen, setAboutOpen] = useState(false);
  const [resetOpen, setResetOpen] = useState(false);
  const [locale, setLocale] = useState<Locale>(localeFromDevice);
  const [drag, setDrag] = useState<DragState | null>(null);
  const [signedIn, setSignedIn] = useState(false);
  const [authOpen, setAuthOpen] = useState(false);
  const [account, setAccount] = useState(FAKE_ACCOUNT);
  const [remainingBusy, setRemainingBusy] = useState(false);
  const pendingAuth = useRef(null as 'me' | 'erase' | null);
  const t = copy[locale];
  const tRef = useRef(t);
  tRef.current = t;
  const dragRef = useRef(drag);
  dragRef.current = drag;
  const sheetsRef = useRef(sheets);
  sheetsRef.current = sheets;
  const preparingIds = useRef(new Set<string>());
  const fittedUri = useRef(new Map<string, { uri: string; bytes: number }>());
  const gridRef = useRef(null as View | null);
  const gridFrame = useRef({ x: 0, y: 0, width: 0, height: 0 });
  const { width: windowWidth } = useWindowDimensions();

  useEffect(() => {
    void loadLocale().then(setLocale);
  }, []);

  useEffect(() => {
    const unsubscribe = subscribeUnauthorized(openLogin);
    void loadSession().then(async (session) => {
      if (session == null) {
        return;
      }
      setSignedIn(true);
      setAccount({
        ...FAKE_ACCOUNT,
        name: session.name,
        email: session.email,
        remaining: session.remaining ?? 0,
        provider: session.provider,
      });
      try {
        const me = await getMe();
        setAccount((current) => ({
          ...current,
          name: me.name,
          email: me.email,
          remaining: me.remaining,
        }));
      } catch (error) {
        if (!isUnauthorized(error)) {
          return;
        }
      }
    });
    return unsubscribe;
  }, []);

  useEffect(() => {
    if (!signedIn || tab !== 'me') {
      return;
    }
    refreshRemaining();
  }, [signedIn, tab]);

  useEffect(() => {
    if (phase !== 'processing' || job == null) {
      return;
    }
    let cancelled = false;
    let settled = false;
    let failures = 0;
    const startedAt = Date.now();

    const tick = () => {
      if (Date.now() - startedAt > 10 * 60 * 1000) {
        if (!settled) {
          settled = true;
          setError(tRef.current.clearingTimeout);
          setPhase('editing');
        }
        return;
      }
      void getJob(job.id)
        .then(async (next) => {
          if (cancelled || settled) {
            return;
          }
          failures = 0;
          applyRemaining(next.remaining);
          const partial = next.status === 'failed' && (next.previews?.length ?? 0) > 0;
          if (next.status === 'succeeded' || partial) {
            settled = true;
            try {
              const uris = await loadPreviews(next);
              if (!cancelled) {
                setSheets((current) => {
                  const nextSheets = current.map((sheet) => ({ ...sheet }));
                  const pending: number[] = [];
                  nextSheets.forEach((sheet, index) => {
                    if (sheet.processed == null) {
                      pending.push(index);
                    }
                  });
                  const paths = next.previews ?? [];
                  for (let n = 0; n < uris.length && n < paths.length; n += 1) {
                    const page = previewPage(paths[n]);
                    const sheetIndex = page == null ? null : pending[page];
                    if (sheetIndex == null) {
                      continue;
                    }
                    nextSheets[sheetIndex] = { ...nextSheets[sheetIndex], processed: uris[n] };
                  }
                  return nextSheets;
                });
                if (partial) {
                  const raw = next.error ?? 'model call failed';
                  setError(localizeApiMessage(tRef.current, raw));
                  setPhase('editing');
                } else {
                  setPhase('done');
                }
              }
            } catch (error) {
              if (cancelled) {
                return;
              }
              if (isUnauthorized(error)) {
                openLogin();
                return;
              }
              setError(tRef.current.previewFailed);
              setPhase('editing');
            }
            return;
          }
          if (next.status === 'failed') {
            settled = true;
            const raw = next.error ?? 'model call failed';
            setError(localizeApiMessage(tRef.current, raw));
            setPhase('editing');
          }
        })
        .catch((error) => {
          if (cancelled || settled) {
            return;
          }
          if (isUnauthorized(error)) {
            settled = true;
            openLogin();
            return;
          }
          failures += 1;
          if (failures >= 3) {
            settled = true;
            setError(tRef.current.serverFailed);
            setPhase('editing');
          }
        });
    };

    tick();
    const timer = setInterval(tick, 2000);
    return () => {
      cancelled = true;
      clearInterval(timer);
    };
  }, [phase, job?.id]);

  function addPages(uris: string[]) {
    setError(null);
    setTab('home');
    const room = MAX_PAGES - sheetsRef.current.length;
    const incoming = uris.slice(0, Math.max(room, 0));
    if (incoming.length === 0) {
      return;
    }
    const extras = incoming.map((uri) => {
      const sheet = {
        id: nextSheetId(),
        original: uri,
        processed: null,
        bytes: 0,
      };
      preparingIds.current.add(sheet.id);
      return sheet;
    });
    const next = [...sheetsRef.current, ...extras];
    sheetsRef.current = next;
    setSheets(next);
    void fitAdded(extras);
  }

  async function fitAdded(extras: Sheet[]) {
    await Promise.all(
      extras.map(async (sheet) => {
        try {
          const prepared = await fitForUpload(sheet.original);
          if (!preparingIds.current.has(sheet.id)) {
            return;
          }
          const bytes = await uriBytes(prepared);
          if (!preparingIds.current.has(sheet.id)) {
            return;
          }
          fittedUri.current.set(sheet.id, { uri: prepared, bytes });
          setSheets((list) =>
            list.map((item) => (item.id === sheet.id ? { ...item, original: prepared, bytes } : item)),
          );
        } catch {
          // The selected photo stays on screen even if shrinking it fails.
        } finally {
          preparingIds.current.delete(sheet.id);
        }
      }),
    );
  }

  async function waitUntilFitted() {
    while (preparingIds.current.size > 0) {
      await new Promise((resolve) => setTimeout(resolve, 40));
    }
  }

  function pageUri(sheet: Sheet): string {
    return sheet.processed ?? fittedUri.current.get(sheet.id)?.uri ?? sheet.original;
  }

  function changeLocale(next: Locale) {
    setLocale(next);
    void saveLocale(next);
  }

  async function scan() {
    if (sheets.length >= MAX_PAGES) {
      setError(t.tooManyPages(MAX_PAGES));
      return;
    }
    const permission = await ImagePicker.requestCameraPermissionsAsync();
    if (!permission.granted) {
      setError(t.cameraDenied);
      return;
    }
    let room = MAX_PAGES - sheets.length;
    try {
      while (room > 0) {
        const result = await ImagePicker.launchCameraAsync({
          mediaTypes: ['images'],
          quality: 0.97,
          allowsEditing: false,
        });
        if (result.canceled || result.assets.length === 0) {
          return;
        }
        addPages(result.assets.map((asset) => asset.uri));
        room -= result.assets.length;
      }
    } catch (caught) {
      if (!isCancel(caught)) {
        setError(caught instanceof Error ? caught.message : t.scanFailed);
      }
    }
  }

  async function pickFromLibrary() {
    if (sheets.length >= MAX_PAGES) {
      setError(t.tooManyPages(MAX_PAGES));
      return;
    }
    const result = await ImagePicker.launchImageLibraryAsync({
      mediaTypes: ['images'],
      allowsMultipleSelection: true,
      selectionLimit: MAX_PAGES - sheets.length,
      quality: 1,
      preferredAssetRepresentationMode:
        ImagePicker.UIImagePickerPreferredAssetRepresentationMode.Current,
    });
    if (!result.canceled) {
      addPages(result.assets.map((asset) => asset.uri));
    }
  }

  async function submit() {
    const selected = sheetsRef.current.filter((sheet) => sheet.processed == null);
    if (selected.length === 0) {
      setError(sheetsRef.current.length === 0 ? t.noPages : t.noNewPages);
      return;
    }
    setError(null);
    setJob(null);
    setPhase('processing');
    try {
      await waitUntilFitted();
      const pending = sheetsRef.current
        .filter((sheet) => sheet.processed == null)
        .map((sheet) => fittedUri.current.get(sheet.id)?.uri ?? sheet.original);
      const created = await createJob(pending, crop, binarization);
      applyRemaining(created.remaining);
      setJob({
        id: created.id,
        status: created.status,
        page_count: pending.length,
        error: null,
        remaining: created.remaining,
      });
    } catch (caught) {
      if (isUnauthorized(caught)) {
        openLogin();
        return;
      }
      const message = caught instanceof Error ? localizeApiMessage(t, caught.message) : t.serverFailed;
      setError(message);
      setPhase('editing');
    }
  }

  function openLogin() {
    pendingAuth.current = null;
    setSignedIn(false);
    setAccount(FAKE_ACCOUNT);
    setTab('home');
    setAuthOpen(true);
    setPhase((current) => (current === 'processing' ? 'editing' : current));
  }

  function closeAuth() {
    pendingAuth.current = null;
    setAuthOpen(false);
  }

  function applyRemaining(remaining?: number) {
    if (remaining == null || !Number.isFinite(remaining)) {
      return;
    }
    setAccount((current) => ({ ...current, remaining }));
  }

  function refreshRemaining(showBusy = false) {
    if (showBusy) {
      if (remainingBusy) {
        return;
      }
      setRemainingBusy(true);
    }
    void getMe()
      .then((me) => {
        setAccount((current) => ({
          ...current,
          name: me.name,
          email: me.email,
          remaining: me.remaining,
        }));
      })
      .catch((error) => {
        if (!isUnauthorized(error)) {
          return;
        }
      })
      .finally(() => {
        if (showBusy) {
          setRemainingBusy(false);
        }
      });
  }

  function finishAuth(next?: { name: string; email: string; remaining: number; provider: AuthProvider }) {
    setSignedIn(true);
    setAccount(
      next == null
        ? FAKE_ACCOUNT
        : {
            ...FAKE_ACCOUNT,
            name: next.name,
            email: next.email,
            remaining: next.remaining,
            provider: next.provider,
          },
    );
    setAuthOpen(false);
    if (next != null) {
      saveCurrentSession({
        name: next.name,
        email: next.email,
        remaining: next.remaining,
        provider: next.provider,
      });
    }
    const action = pendingAuth.current;
    pendingAuth.current = null;
    if (action === 'me') {
      setTab('me');
    }
    if (action === 'erase') {
      void submit();
    }
  }

  function requestAuth(action: 'me' | 'erase') {
    if (signedIn && authHeaders().Authorization != null) {
      if (action === 'me') {
        setTab('me');
      }
      if (action === 'erase') {
        void submit();
      }
      return;
    }
    pendingAuth.current = action;
    if (signedIn) {
      notifyUnauthorized();
      return;
    }
    setAuthOpen(true);
  }

  async function sharePdf() {
    await waitUntilFitted();
    const uris = sheetsRef.current.map((sheet) => pageUri(sheet));
    if (uris.length === 0) {
      setError(t.noPages);
      return;
    }
    setSharing(true);
    setError(null);
    try {
      const directory = cacheDirectory;
      if (directory == null) {
        setError(t.saveFailed);
        return;
      }
      const destination = `${directory}exam.pdf`;
      await writeExamPdf(uris, destination);
      if (!(await Sharing.isAvailableAsync())) {
        setError(t.shareUnavailable);
        return;
      }
      await Sharing.shareAsync(destination, {
        mimeType: 'application/pdf',
        UTI: 'com.adobe.pdf',
      });
    } catch (caught) {
      if (isUnauthorized(caught)) {
        openLogin();
        return;
      }
      setError(caught instanceof Error ? localizeApiMessage(t, caught.message) : t.pdfFailed);
    } finally {
      setSharing(false);
    }
  }

  function reset() {
    preparingIds.current.clear();
    fittedUri.current.clear();
    sheetsRef.current = [];
    setSheets([]);
    setJob(null);
    setError(null);
    setSelectedPage(null);
    setPhase('editing');
    setTab('home');
  }

  function confirmReset() {
    setResetOpen(true);
  }

  function closeReset() {
    setResetOpen(false);
  }

  function applyReset() {
    setResetOpen(false);
    reset();
  }

  function removeSheet(index: number) {
    const removed = sheets[index];
    if (removed != null) {
      preparingIds.current.delete(removed.id);
      fittedUri.current.delete(removed.id);
    }
    const next = sheets.filter((_, item) => item !== index);
    sheetsRef.current = next;
    setSheets(next);
    if (next.length === 0) {
      setPhase('editing');
      setJob(null);
    }
  }

  function measureGrid() {
    gridRef.current?.measureInWindow((x, y, width, height) => {
      gridFrame.current = { x, y, width, height };
    });
  }

  function indexAtPoint(pageX: number, pageY: number, count: number): number {
    const frame = gridFrame.current;
    if (frame.width <= 0 || count <= 0) {
      return 0;
    }
    const cell = frame.width / 3;
    const col = Math.min(2, Math.max(0, Math.floor((pageX - frame.x) / cell)));
    const row = Math.max(0, Math.floor((pageY - frame.y) / cell));
    return Math.min(count - 1, Math.max(0, row * 3 + col));
  }

  function onDragMove(event: GestureResponderEvent) {
    const current = dragRef.current;
    if (current == null) {
      return;
    }
    const { pageX, pageY } = event.nativeEvent;
    const to = indexAtPoint(pageX, pageY, sheetsRef.current.length);
    if (to !== current.from) {
      setSheets((list) => moveItem(list, current.from, to));
    }
    const next = { ...current, from: to, x: pageX, y: pageY };
    dragRef.current = next;
    setDrag(next);
  }

  function endDrag() {
    if (dragRef.current == null) {
      return;
    }
    dragRef.current = null;
    setDrag(null);
  }

  const busy = phase === 'processing';
  const processedCount = sheets.filter((sheet) => sheet.processed != null).length;
  const pendingCount = sheets.length - processedCount;
  const totalBytes = sheets.reduce((sum, sheet) => sum + sheet.bytes, 0);
  const totalKb = Math.max(0, Math.round(totalBytes / 1024));
  const shownPages = sheets.map((sheet) => sheet.processed ?? sheet.original);
  const showBanner = tab === 'home' && phase !== 'processing' && processedCount === 0;
  const showResult = tab === 'home' && processedCount > 0 && phase !== 'processing';
  const showTiles = tab === 'home' && phase !== 'processing' && processedCount === 0;
  const canReorder = showResult && sheets.length > 1;

  function addFromAlbum() {
    if (Platform.OS === 'ios') {
      void pickFromLibrary();
      return;
    }
    void scan();
  }

  if (selectedPage != null) {
    return (
      <ImageViewer
        key={sheets.map((sheet) => sheet.id).join('|')}
        pages={shownPages}
        originals={sheets.map((sheet) => sheet.original)}
        startIndex={selectedPage.index}
        backLabel={t.back}
        pageLabel={t.pageCount}
        seeBefore={t.seeBefore}
        seeAfter={t.seeAfter}
        beforeLabel={t.before}
        afterLabel={t.after}
        onBack={() => setSelectedPage(null)}
      />
    );
  }

  return (
    <View style={styles.screen}>
      <StatusBar style="dark" />
      <View style={styles.blobYellow} />
      <View style={styles.blobPink} />
      <View style={styles.blobPeach} />

      {error != null && tab === 'home' ? (
        <View style={styles.errorBanner}>
          <View style={styles.errorBannerCopy}>
            <Text style={styles.errorBannerText}>{error}</Text>
            {job ? (
              <Text selectable style={styles.errorBannerJob}>
                {t.jobId(job.id)}
              </Text>
            ) : null}
          </View>
          <Pressable
            onPress={() => setError(null)}
            hitSlop={8}
            accessibilityRole="button"
            accessibilityLabel={t.close}
          >
            <Text style={styles.errorBannerClose}>×</Text>
          </Pressable>
        </View>
      ) : null}

      <ScrollView
        contentContainerStyle={styles.bodyScroll}
        showsVerticalScrollIndicator={false}
        stickyHeaderIndices={showTiles || showResult ? [1] : []}
        scrollEnabled={drag == null}
      >
        <View style={styles.headerBlock}>
          <View style={styles.brandRow}>
            <View style={styles.logo}>
              <View style={styles.logoFace}>
                <View style={styles.logoEye} />
                <View style={styles.logoEye} />
              </View>
              <View style={styles.logoSmile} />
            </View>
            <Text style={styles.title}>空白卷</Text>
          </View>
          <View style={styles.subtitleRow}>
            <Text style={styles.subtitle}>{t.subtitle}</Text>
            <View style={styles.langRow}>
              {LOCALES.map((item) => {
                const on = item.id === locale;
                return (
                  <Pressable
                    key={item.id}
                    style={[styles.langChip, on ? styles.langChipOn : null]}
                    onPress={() => changeLocale(item.id)}
                    hitSlop={6}
                    accessibilityRole="button"
                    accessibilityState={{ selected: on }}
                    accessibilityLabel={item.label}
                  >
                    <Text style={[styles.langChipLabel, on ? styles.langChipLabelOn : null]}>{item.label}</Text>
                  </Pressable>
                );
              })}
            </View>
          </View>

          {showBanner ? (
            <Pressable style={styles.banner} onPress={() => setAboutOpen(true)}>
              <View style={styles.bannerCopy}>
                <Text style={styles.bannerTitle}>{t.bannerTitle}</Text>
                <Text style={styles.bannerHint}>{t.bannerHint}</Text>
                <View style={styles.bannerChip}>
                  <Text style={styles.bannerChipLabel}>{t.learnMore}</Text>
                </View>
              </View>
              <View style={styles.bannerPage}>
                <View style={styles.bannerLineWide} />
                <View style={styles.bannerLine} />
                <View style={styles.bannerLineShort} />
                <View style={styles.bannerRule} />
                <View style={styles.bannerRule} />
              </View>
            </Pressable>
          ) : null}
        </View>

        {showResult ? (
          <View style={styles.tilesSticky}>
            <View style={styles.resultCard}>
              <Text style={styles.resultTitle}>{t.erased(processedCount)}</Text>
              <Text style={styles.resultHint}>
                {pendingCount > 0 ? t.pendingLeft(pendingCount) : t.erasedHint}
              </Text>
              {pendingCount > 0 ? (
                <>
                  <Pressable style={styles.resultPrimary} disabled={busy} onPress={() => requestAuth('erase')}>
                    <Text style={styles.resultPrimaryLabel}>{t.eraseNew}</Text>
                  </Pressable>
                  <FlagSwitch
                    label={t.crop}
                    value={crop}
                    disabled={busy}
                    onToggle={() => setCrop((current) => !current)}
                    onHelp={() => setOptionHelp({ title: t.cropHelpTitle, copy: t.cropHelp })}
                    helpLabel={t.cropHelpTitle}
                    hintDark
                    rowStyle={styles.resultCropRow}
                    labelStyle={styles.resultCropLabel}
                  />
                  <FlagSwitch
                    label={t.binarization}
                    value={binarization}
                    disabled={busy}
                    onToggle={() => setBinarization((current) => !current)}
                    onHelp={() => setOptionHelp({ title: t.binarizationHelpTitle, copy: t.binarizationHelp })}
                    helpLabel={t.binarizationHelpTitle}
                    hintDark
                    rowStyle={styles.resultCropRow}
                    labelStyle={styles.resultCropLabel}
                  />
                </>
              ) : (
                <Pressable
                  style={styles.resultPrimary}
                  disabled={sharing}
                  onPress={() => void sharePdf()}
                >
                  <Text style={styles.resultPrimaryLabel}>{sharing ? t.preparingPdf : t.downloadPdf}</Text>
                </Pressable>
              )}
            </View>
            <View style={styles.resultRow}>
              <Pressable style={styles.resultSecondary} onPress={addFromAlbum}>
                <AddPagesIcon />
                <Text style={styles.resultSecondaryLabel}>{t.addMore}</Text>
              </Pressable>
              {pendingCount > 0 ? (
                <Pressable
                  style={styles.resultSecondary}
                  disabled={sharing}
                  onPress={() => void sharePdf()}
                >
                  <Text style={styles.resultSecondaryLabel}>{sharing ? t.preparingPdf : t.downloadPdf}</Text>
                </Pressable>
              ) : (
                <Pressable style={styles.resultSecondary} onPress={confirmReset}>
                  <AgainIcon />
                  <Text style={styles.resultSecondaryLabel}>{t.again}</Text>
                </Pressable>
              )}
            </View>
            {pendingCount > 0 ? (
              <Pressable style={styles.resultAgain} onPress={confirmReset}>
                <AgainIcon color="#6E6E6E" />
                <Text style={styles.resultAgainLabel}>{t.again}</Text>
              </Pressable>
            ) : null}
          </View>
        ) : null}

        {showTiles ? (
          <View style={styles.tilesSticky}>
            <View style={styles.tiles}>
                <Pressable style={styles.tile} disabled={busy} onPress={addFromAlbum}>
                  <View>
                    <Text style={styles.tileTitle}>{t.album}</Text>
                    <Text style={styles.tileCopy}>
                      {Platform.OS === 'ios' ? t.maxPages(MAX_PAGES) : t.albumAndroid}
                    </Text>
                  </View>
                  <View style={[styles.iconBtn, styles.pillPink]}>
                    <View style={styles.plusH} />
                    <View style={styles.plusV} />
                  </View>
                </Pressable>
                <View style={[styles.tile, pendingCount === 0 || busy ? styles.tileDisabled : null]}>
                  <View style={styles.tileHead}>
                    <Pressable disabled={busy} onPress={() => requestAuth('erase')} style={styles.tileHeadCopy}>
                      <Text style={styles.tileTitle}>{t.erase}</Text>
                      <Text style={styles.tileCopy}>{processedCount > 0 ? t.eraseNew : t.keepPrint}</Text>
                    </Pressable>
                    <Pressable
                      style={[styles.iconBtn, styles.pillPurple, styles.tileHeadAction]}
                      disabled={busy}
                      onPress={() => requestAuth('erase')}
                    >
                      <View style={styles.play} />
                    </Pressable>
                  </View>
                  <FlagSwitch
                    label={t.crop}
                    value={crop}
                    disabled={busy}
                    onToggle={() => setCrop((current) => !current)}
                    onHelp={() => setOptionHelp({ title: t.cropHelpTitle, copy: t.cropHelp })}
                    helpLabel={t.cropHelpTitle}
                    rowStyle={styles.cropRow}
                    labelStyle={styles.cropLabel}
                  />
                  <FlagSwitch
                    label={t.binarization}
                    value={binarization}
                    disabled={busy}
                    onToggle={() => setBinarization((current) => !current)}
                    onHelp={() => setOptionHelp({ title: t.binarizationHelpTitle, copy: t.binarizationHelp })}
                    helpLabel={t.binarizationHelpTitle}
                    rowStyle={styles.cropRow}
                    labelStyle={styles.cropLabel}
                  />
                </View>
            </View>
          </View>
        ) : null}

        <View style={styles.mainBlock}>
          {tab === 'me' ? (
            <View style={styles.mePage}>
              <View style={styles.meProfile}>
                <View style={styles.meAvatarWrap}>
                  <View style={styles.meAvatar}>
                    <View style={styles.meFace}>
                      <View style={styles.logoEye} />
                      <View style={styles.logoEye} />
                    </View>
                    <View style={styles.meSmile} />
                  </View>
                  {account.provider != null ? (
                    <View
                      style={[
                        styles.meProviderBadge,
                        account.provider === 'apple' ? styles.meAppleBadge : null,
                      ]}
                      accessibilityLabel={account.provider === 'apple' ? 'Apple' : 'Google'}
                    >
                      {account.provider === 'apple' ? (
                        <Text style={styles.meAppleMark}></Text>
                      ) : (
                        <GoogleMark />
                      )}
                    </View>
                  ) : null}
                </View>
                <View style={styles.meProfileCopy}>
                  <Text style={styles.meName}>{account.name}</Text>
                  <Text style={styles.meEmail}>{account.email}</Text>
                  <View style={styles.meBadge}>
                    <Text style={styles.meBadgeLabel}>{t.signedInBadge}</Text>
                  </View>
                </View>
              </View>
              <View style={styles.meTable}>
                <Pressable
                  style={styles.meRow}
                  onPress={() => refreshRemaining(true)}
                  disabled={remainingBusy}
                  accessibilityRole="button"
                  accessibilityLabel={t.remainingErase}
                >
                  <Text style={styles.meRowLabel}>{t.remainingErase}</Text>
                  <View style={styles.meRefreshWrap}>
                    {remainingBusy ? (
                      <ActivityIndicator color="#6E6E6E" size="small" />
                    ) : (
                      <RefreshMark />
                    )}
                  </View>
                  <View style={styles.meRowValue}>
                    <Text style={styles.meCount}>{account.remaining}</Text>
                    {t.remainingTimes.length > 0 ? (
                      <Text style={styles.meCountUnit}>{t.remainingTimes}</Text>
                    ) : null}
                    <Text style={styles.meChevron}>›</Text>
                  </View>
                </Pressable>
                <View style={styles.meDivider} />
                <Pressable
                  style={styles.meRow}
                  onPress={() => {
                    clearAccessToken();
                    setSignedIn(false);
                    setAccount(FAKE_ACCOUNT);
                    setTab('home');
                  }}
                >
                  <Text style={styles.meRowLabel}>{t.logout}</Text>
                </Pressable>
              </View>
              <Text style={styles.meCopy}>{t.meCopy}</Text>
            </View>
          ) : (
            <>
              {shownPages.length === 0 && phase === 'editing' ? (
                <Text style={styles.empty}>{t.empty}</Text>
              ) : null}

              {shownPages.length > 0 && totalKb > 0 ? (
                <Text style={styles.pageSize}>{t.totalSize(totalKb)}</Text>
              ) : null}

              {shownPages.length > 0 ? (
                <View
                  ref={gridRef}
                  collapsable={false}
                  style={styles.pageGrid}
                  onLayout={measureGrid}
                >
                  {shownPages.map((uri, index) => {
                    const sheet = sheets[index];
                    if (sheet == null) {
                      return null;
                    }
                    const dragging = drag?.id === sheet.id;
                    return (
                    <View key={sheet.id} style={[styles.gridCell, dragging ? styles.gridCellDragging : null]}>
                      <Pressable
                        style={styles.gridThumb}
                        delayLongPress={220}
                        onPress={() => setSelectedPage({ uri, index })}
                        onLongPress={
                          canReorder && sheet.processed != null
                            ? (event) => {
                                const { pageX, pageY } = event.nativeEvent;
                                const next = {
                                  id: sheet.id,
                                  from: index,
                                  x: pageX,
                                  y: pageY,
                                  uri,
                                };
                                dragRef.current = next;
                                setDrag(next);
                                gridRef.current?.measureInWindow((x, y, width, height) => {
                                  gridFrame.current = { x, y, width, height };
                                });
                              }
                            : undefined
                        }
                        onTouchMove={onDragMove}
                        onTouchEnd={endDrag}
                        onTouchCancel={endDrag}
                      >
                        <Image source={{ uri }} style={styles.gridImage} resizeMode="cover" />
                        <View style={styles.gridIndex}>
                          <Text style={styles.gridIndexLabel}>{index + 1}</Text>
                        </View>
                      </Pressable>
                      {sheet.processed != null ? (
                        <View style={styles.gridDoneDot} />
                      ) : null}
                      {phase !== 'processing' && sheet.processed == null && processedCount > 0 ? (
                        <View style={styles.gridPending}>
                          <Text style={styles.gridPendingLabel}>{t.pending}</Text>
                        </View>
                      ) : null}
                      {phase !== 'processing' && drag == null ? (
                        <Pressable
                          style={styles.gridRemove}
                          onPress={() => removeSheet(index)}
                          hitSlop={6}
                        >
                          <Text style={styles.gridRemoveLabel}>×</Text>
                        </Pressable>
                      ) : null}
                    </View>
                    );
                  })}
                </View>
              ) : null}

              {phase === 'processing' ? (
                <View>
                  <View style={styles.statusRow}>
                    <ActivityIndicator color="#111111" />
                    <Text style={styles.status}>{t.clearing}</Text>
                  </View>
                  {job ? (
                    <Text selectable style={styles.jobId}>
                      {t.jobId(job.id)}
                    </Text>
                  ) : null}
                </View>
              ) : null}

            </>
          )}

        </View>
      </ScrollView>

      {drag != null ? (
        <View
          pointerEvents="none"
          style={[
            styles.dragGhost,
            {
              width: Math.max(72, (windowWidth - 40) / 3 - 8),
              height: Math.max(72, (windowWidth - 40) / 3 - 8),
              left: drag.x - Math.max(72, (windowWidth - 40) / 3 - 8) / 2,
              top: drag.y - Math.max(72, (windowWidth - 40) / 3 - 8) / 2,
            },
          ]}
        >
          <Image source={{ uri: drag.uri }} style={styles.dragGhostImage} resizeMode="cover" />
        </View>
      ) : null}

      <View style={styles.tabDock}>
        <View style={styles.tabBar}>
          <Pressable style={styles.tabItem} onPress={() => setTab('home')}>
            <View style={styles.homeGrid}>
              <View style={[styles.homeDot, tab === 'home' ? styles.tabIconOn : styles.tabIconOff]} />
              <View style={[styles.homeDot, tab === 'home' ? styles.tabIconOn : styles.tabIconOff]} />
              <View style={[styles.homeDot, tab === 'home' ? styles.tabIconOn : styles.tabIconOff]} />
              <View style={[styles.homeDot, tab === 'home' ? styles.tabIconOn : styles.tabIconOff]} />
            </View>
            <Text style={[styles.tabLabel, tab === 'home' ? styles.tabOn : null]}>{t.home}</Text>
          </Pressable>
          <View style={styles.tabCamSlot} />
          <Pressable style={styles.tabItem} onPress={() => requestAuth('me')}>
            <View style={styles.personIcon}>
              <View style={[styles.personHead, tab === 'me' ? styles.tabIconOnBorder : styles.tabIconOffBorder]} />
              <View style={[styles.personBody, tab === 'me' ? styles.tabIconOnBorder : styles.tabIconOffBorder]} />
            </View>
            <Text style={[styles.tabLabel, tab === 'me' ? styles.tabOn : null]}>{t.me}</Text>
          </Pressable>
        </View>
        <Pressable style={styles.tabCamWrap} disabled={busy} onPress={() => void scan()}>
          <View style={styles.tabCam}>
            <View style={styles.tabCamGlyph}>
              <View style={styles.tabCamFlash} />
              <View style={styles.tabCamBody}>
                <View style={styles.tabCamLens} />
              </View>
            </View>
          </View>
        </Pressable>
      </View>

      <Modal
        visible={resetOpen}
        transparent
        animationType="fade"
        onRequestClose={closeReset}
      >
        <View style={styles.helpScrim}>
          <Pressable style={styles.confirmDismiss} onPress={closeReset} />
          <View style={styles.helpCard}>
            <Text style={styles.helpTitle}>{t.againConfirm}</Text>
            <Text style={styles.helpCopy}>{t.againConfirmCopy}</Text>
            <View style={styles.helpActions}>
              <Pressable style={styles.helpCancel} onPress={closeReset}>
                <Text style={styles.helpCancelLabel}>{t.cancel}</Text>
              </Pressable>
              <Pressable style={styles.helpAction} onPress={applyReset}>
                <Text style={styles.confirmOkLabel}>{t.confirm}</Text>
              </Pressable>
            </View>
          </View>
          <Pressable style={styles.confirmDismiss} onPress={closeReset} />
        </View>
      </Modal>

      <Modal
        visible={optionHelp != null}
        transparent
        animationType="fade"
        onRequestClose={() => setOptionHelp(null)}
      >
        <View style={styles.helpScrim}>
          <Pressable style={styles.confirmDismiss} onPress={() => setOptionHelp(null)} />
          <View style={styles.helpCard}>
            <Text style={styles.helpTitle}>{optionHelp?.title}</Text>
            <Text style={styles.helpCopy}>{optionHelp?.copy}</Text>
            <Pressable style={styles.helpClose} onPress={() => setOptionHelp(null)}>
              <Text style={styles.confirmOkLabel}>{t.aboutClose}</Text>
            </Pressable>
          </View>
          <Pressable style={styles.confirmDismiss} onPress={() => setOptionHelp(null)} />
        </View>
      </Modal>

      <Modal
        visible={aboutOpen}
        transparent
        animationType="slide"
        onRequestClose={() => setAboutOpen(false)}
      >
        <View style={styles.aboutScrim}>
          <Pressable style={styles.aboutDismiss} onPress={() => setAboutOpen(false)} />
          <View style={styles.aboutPanel}>
            <View style={styles.aboutHandle} />
            <Text style={styles.aboutTitle}>{t.aboutTitle}</Text>
            <ScrollView
              style={styles.aboutList}
              showsVerticalScrollIndicator={false}
              bounces={false}
            >
              {t.aboutFeatures(MAX_PAGES).map((item) => (
                <View key={item} style={styles.aboutRow}>
                  <View style={styles.aboutDot} />
                  <Text style={styles.aboutItem}>{item}</Text>
                </View>
              ))}
            </ScrollView>
            <Pressable style={styles.aboutClose} onPress={() => setAboutOpen(false)}>
              <Text style={styles.aboutCloseLabel}>{t.aboutClose}</Text>
            </Pressable>
          </View>
        </View>
      </Modal>

      <AuthModal
        visible={authOpen}
        locale={locale}
        onChangeLocale={changeLocale}
        onClose={closeAuth}
        onSignedIn={finishAuth}
      />
    </View>
  );
}

function previewPage(path: string): number | null {
  const matched = /\/preview\/(\d+)$/.exec(path);
  if (matched == null) {
    return null;
  }
  const page = Number(matched[1]);
  return Number.isInteger(page) ? page : null;
}

async function loadPreviews(job: Job): Promise<string[]> {
  const paths = job.previews ?? [];
  if (cacheDirectory == null) {
    throw new Error('無法儲存檔案');
  }
  const uris: string[] = [];
  for (const [index, path] of paths.entries()) {
    const destination = `${cacheDirectory}exam-${job.id}-${index}.jpg`;
    const downloaded = await downloadAsync(`${API_BASE_URL}${path}`, destination, {
      headers: authHeaders(),
    });
    const contentType =
      downloaded.mimeType ??
      downloaded.headers?.['Content-Type'] ??
      downloaded.headers?.['content-type'] ??
      '';
    if (contentType.includes('application/json')) {
      const raw = await readAsStringAsync(downloaded.uri);
      failFromEnvelope(JSON.parse(raw) as { code?: number; reason?: string });
    }
    uris.push(downloaded.uri);
  }
  return uris;
}

function isCancel(error: unknown): boolean {
  const message = error instanceof Error ? error.message : String(error);
  return /cancel/i.test(message);
}

function AddPagesIcon() {
  return (
    <View style={styles.addIcon}>
      <View style={styles.addIconH} />
      <View style={styles.addIconV} />
    </View>
  );
}

function AgainIcon({ color = '#111111' }: { color?: string }) {
  return (
    <View style={styles.againIcon}>
      <View style={[styles.againArc, { borderColor: color, borderTopColor: 'transparent' }]} />
      <View style={[styles.againHead, { borderBottomColor: color }]} />
    </View>
  );
}

const VIEWABILITY = { itemVisiblePercentThreshold: 60 };
const THUMB_WIDTH = 56;
const THUMB_GAP = 8;
const THUMB_STRIDE = THUMB_WIDTH + THUMB_GAP;

function ImageViewer({
  pages,
  originals,
  startIndex,
  backLabel,
  pageLabel,
  seeBefore,
  seeAfter,
  beforeLabel,
  afterLabel,
  onBack,
}: {
  pages: string[];
  originals: string[];
  startIndex: number;
  backLabel: string;
  pageLabel: (n: number, total: number) => string;
  seeBefore: string;
  seeAfter: string;
  beforeLabel: string;
  afterLabel: string;
  onBack: () => void;
}) {
  const { width } = useWindowDimensions();
  const [current, setCurrent] = useState(startIndex);
  const [showBefore, setShowBefore] = useState(false);
  const currentCanCompare =
    originals[current] != null && pages[current] != null && originals[current] !== pages[current];
  const pagerRef = useRef(null as FlatList<string> | null);
  const thumbRef = useRef(null as FlatList<string> | null);
  const skipThumbSync = useRef(true);
  const onViewableItemsChanged = useRef(({ viewableItems }: { viewableItems: ViewToken[] }) => {
    const next = viewableItems[0]?.index;
    if (next != null) {
      setCurrent(next);
    }
  }).current;

  useEffect(() => {
    if (skipThumbSync.current) {
      skipThumbSync.current = false;
      return;
    }
    if (current < 0 || current >= pages.length) {
      return;
    }
    try {
      thumbRef.current?.scrollToIndex({
        index: current,
        animated: true,
        viewPosition: 0.5,
      });
    } catch {
      // List may not be measured yet.
    }
  }, [current, pages.length]);

  function jumpTo(index: number) {
    if (index < 0 || index >= pages.length) {
      return;
    }
    setCurrent(index);
    try {
      pagerRef.current?.scrollToIndex({ index, animated: true });
    } catch {
      // Ignore a jump before layout is ready.
    }
  }

  return (
    <View style={styles.viewer}>
      <StatusBar style="light" />
      <View style={styles.viewerHeader}>
        <Pressable style={styles.backButton} onPress={onBack}>
          <Text style={styles.backLabel}>{backLabel}</Text>
        </Pressable>
        <Text style={styles.viewerTitle}>{pageLabel(current + 1, pages.length)}</Text>
        {currentCanCompare ? (
          <Pressable style={styles.viewerCompare} onPress={() => setShowBefore((current) => !current)}>
            <Text style={styles.viewerCompareLabel}>{showBefore ? seeAfter : seeBefore}</Text>
          </Pressable>
        ) : (
          <View style={styles.backSpacer} />
        )}
      </View>
      {currentCanCompare ? (
        <View style={styles.viewerMode} pointerEvents="none">
          <Text style={styles.viewerModeLabel}>{showBefore ? beforeLabel : afterLabel}</Text>
        </View>
      ) : null}
      <FlatList
        ref={pagerRef}
        style={styles.viewerPager}
        data={pages}
        extraData={showBefore}
        horizontal
        pagingEnabled
        initialScrollIndex={startIndex}
        getItemLayout={(_, index) => ({ length: width, offset: width * index, index })}
        keyExtractor={(_, index) => String(index)}
        renderItem={({ item, index }) => {
          const uri = showBefore && originals[index] != null ? originals[index] : item;
          return (
            <View style={{ width, flex: 1 }} collapsable={false}>
              <ZoomableImage key={uri} uri={uri} />
            </View>
          );
        }}
        onViewableItemsChanged={onViewableItemsChanged}
        viewabilityConfig={VIEWABILITY}
        showsHorizontalScrollIndicator={false}
        bounces={pages.length > 1}
        initialNumToRender={pages.length}
        windowSize={Math.max(3, pages.length)}
        removeClippedSubviews={false}
        onScrollToIndexFailed={() => undefined}
      />
      <FlatList
        ref={thumbRef}
        data={pages}
        horizontal
        style={styles.thumbStrip}
        contentContainerStyle={styles.thumbStripContent}
        initialScrollIndex={startIndex}
        getItemLayout={(_, index) => ({ length: THUMB_STRIDE, offset: THUMB_STRIDE * index, index })}
        extraData={pages}
        keyExtractor={(_, index) => `thumb-${index}`}
        showsHorizontalScrollIndicator={false}
        initialNumToRender={pages.length}
        onScrollToIndexFailed={() => undefined}
        renderItem={({ item, index }) => {
          const on = index === current;
          return (
            <Pressable
              onPress={() => jumpTo(index)}
              style={[styles.thumb, on ? styles.thumbOn : null]}
            >
              <Image source={{ uri: item }} style={styles.thumbImage} resizeMode="cover" />
              <View style={styles.thumbIndex}>
                <Text style={styles.thumbIndexLabel}>{index + 1}</Text>
              </View>
            </Pressable>
          );
        }}
      />
    </View>
  );
}

function pinchDistance(event: GestureResponderEvent): number {
  const touches = event.nativeEvent.touches;
  if (touches.length < 2) {
    return 0;
  }
  return Math.hypot(touches[0].pageX - touches[1].pageX, touches[0].pageY - touches[1].pageY);
}

function ZoomableImage({ uri }: { uri: string }) {
  const [box, setBox] = useState({ width: 0, height: 0 });

  return (
    <View
      style={styles.viewerStage}
      onLayout={(event) => {
        const { width, height } = event.nativeEvent.layout;
        setBox((current) =>
          current.width === width && current.height === height ? current : { width, height },
        );
      }}
    >
      {box.width === 0 ? null : <PinchImage uri={uri} width={box.width} height={box.height} />}
    </View>
  );
}

function PinchImage({ uri, width, height }: { uri: string; width: number; height: number }) {
  const scale = useRef(1);
  const tx = useRef(0);
  const ty = useRef(0);
  const start = useRef({ scale: 1, x: 0, y: 0, distance: 0 });
  const [transform, setTransform] = useState({ scale: 1, x: 0, y: 0 });

  const responder = useRef(
    PanResponder.create({
      onStartShouldSetPanResponder: (event) => event.nativeEvent.touches.length >= 2,
      onMoveShouldSetPanResponder: (event) =>
        event.nativeEvent.touches.length >= 2 || scale.current > 1,
      onPanResponderTerminationRequest: () => false,
      onPanResponderGrant: (event) => {
        start.current = {
          scale: scale.current,
          x: tx.current,
          y: ty.current,
          distance: pinchDistance(event),
        };
      },
      onPanResponderMove: (event, gesture) => {
        const distance = pinchDistance(event);
        let nextScale = scale.current;
        let nextX = tx.current;
        let nextY = ty.current;
        if (distance > 0) {
          if (start.current.distance === 0) {
            start.current.distance = distance;
            start.current.scale = scale.current;
          }
          nextScale = start.current.scale * (distance / start.current.distance);
          nextX = start.current.x;
          nextY = start.current.y;
        } else if (scale.current > 1) {
          nextScale = scale.current;
          nextX = start.current.x + gesture.dx;
          nextY = start.current.y + gesture.dy;
        }
        nextScale = Math.min(5, Math.max(1, nextScale));
        if (nextScale === 1) {
          nextX = 0;
          nextY = 0;
        }
        scale.current = nextScale;
        tx.current = nextX;
        ty.current = nextY;
        setTransform({ scale: nextScale, x: nextX, y: nextY });
      },
      onPanResponderRelease: () => {
        if (scale.current <= 1.02) {
          scale.current = 1;
          tx.current = 0;
          ty.current = 0;
          setTransform({ scale: 1, x: 0, y: 0 });
        }
      },
    }),
  ).current;

  return (
    <View style={styles.viewerZoom} {...responder.panHandlers}>
      <Image
        source={{ uri }}
        style={{
          width,
          height,
          transform: [
            { translateX: transform.x },
            { translateY: transform.y },
            { scale: transform.scale },
          ],
        }}
        resizeMode="contain"
      />
    </View>
  );
}

const styles = StyleSheet.create({
  screen: {
    flex: 1,
    backgroundColor: '#FFF7EC',
  },
  blobYellow: {
    position: 'absolute',
    top: -80,
    left: -60,
    width: 280,
    height: 280,
    borderRadius: 140,
    backgroundColor: '#FFD600',
    opacity: 0.55,
  },
  blobPink: {
    position: 'absolute',
    top: -40,
    right: -90,
    width: 300,
    height: 300,
    borderRadius: 150,
    backgroundColor: '#FF7A59',
    opacity: 0.28,
  },
  blobPeach: {
    position: 'absolute',
    top: 220,
    right: -70,
    width: 240,
    height: 240,
    borderRadius: 120,
    backgroundColor: '#FF5AAF',
    opacity: 0.12,
  },
  bodyScroll: {
    paddingBottom: 140,
  },
  headerBlock: {
    paddingTop: 64,
    paddingHorizontal: 20,
  },
  tilesSticky: {
    paddingTop: 30,
    paddingHorizontal: 20,
    paddingBottom: 8,
    backgroundColor: '#FFF7EC',
  },
  mainBlock: {
    paddingHorizontal: 20,
  },
  brandRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 12,
  },
  logo: {
    width: 44,
    height: 44,
    borderRadius: 14,
    backgroundColor: '#111111',
    alignItems: 'center',
    justifyContent: 'center',
  },
  logoFace: {
    width: 26,
    height: 26,
    borderRadius: 13,
    backgroundColor: '#FFD600',
    flexDirection: 'row',
    justifyContent: 'space-evenly',
    alignItems: 'center',
    paddingTop: 4,
  },
  logoEye: {
    width: 4,
    height: 4,
    borderRadius: 2,
    backgroundColor: '#111111',
  },
  logoSmile: {
    width: 10,
    height: 5,
    borderBottomLeftRadius: 8,
    borderBottomRightRadius: 8,
    borderWidth: 2,
    borderTopWidth: 0,
    borderColor: '#111111',
    marginTop: -8,
  },
  title: {
    fontSize: 34,
    fontWeight: '800',
    color: '#111111',
  },
  subtitleRow: {
    marginTop: 8,
    marginBottom: 20,
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: 8,
  },
  subtitle: {
    flex: 1,
    fontSize: 16,
    color: '#5A5A5A',
    paddingRight: 8,
  },
  langRow: {
    flexDirection: 'row',
    gap: 8,
  },
  langChip: {
    minWidth: 44,
    height: 36,
    paddingHorizontal: 12,
    borderRadius: 12,
    backgroundColor: '#FFFFFF',
    alignItems: 'center',
    justifyContent: 'center',
  },
  langChipOn: {
    backgroundColor: '#FFD600',
  },
  langChipLabel: {
    fontSize: 15,
    fontWeight: '700',
    color: '#6E6E6E',
  },
  langChipLabelOn: {
    color: '#111111',
  },
  banner: {
    backgroundColor: '#111111',
    borderRadius: 28,
    minHeight: 168,
    paddingHorizontal: 22,
    paddingVertical: 22,
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  bannerCopy: {
    flex: 1,
    paddingRight: 12,
  },
  bannerTitle: {
    color: '#FFFFFF',
    fontSize: 32,
    fontWeight: '800',
  },
  bannerHint: {
    marginTop: 8,
    color: '#FFD600',
    fontSize: 15,
    fontWeight: '700',
  },
  bannerChip: {
    marginTop: 16,
    alignSelf: 'flex-start',
    backgroundColor: '#FFD600',
    borderRadius: 16,
    paddingHorizontal: 14,
    paddingVertical: 8,
  },
  bannerChipLabel: {
    color: '#111111',
    fontWeight: '800',
    fontSize: 14,
  },
  bannerPage: {
    width: 88,
    height: 108,
    borderRadius: 12,
    backgroundColor: '#FFFFFF',
    padding: 12,
    justifyContent: 'flex-start',
    gap: 8,
  },
  bannerLineWide: {
    height: 8,
    borderRadius: 4,
    backgroundColor: '#EEEEEE',
  },
  bannerLine: {
    height: 8,
    width: '70%',
    borderRadius: 4,
    backgroundColor: '#EEEEEE',
  },
  bannerLineShort: {
    height: 8,
    width: '50%',
    borderRadius: 4,
    backgroundColor: '#EEEEEE',
  },
  bannerRule: {
    height: 2,
    backgroundColor: '#DDDDDD',
    marginTop: 6,
  },
  tiles: {
    flexDirection: 'row',
    gap: 12,
  },
  tile: {
    flex: 1,
    backgroundColor: '#FFFFFF',
    borderRadius: 24,
    padding: 18,
    minHeight: 150,
    justifyContent: 'space-between',
  },
  tileDisabled: {
    opacity: 0.55,
  },
  tileTitle: {
    fontSize: 22,
    fontWeight: '800',
    color: '#111111',
  },
  tileCopy: {
    marginTop: 8,
    fontSize: 14,
    color: '#6E6E6E',
  },
  tileHead: {
    flexDirection: 'row',
    alignItems: 'flex-start',
    justifyContent: 'space-between',
    gap: 8,
  },
  tileHeadCopy: {
    flex: 1,
  },
  tileHeadAction: {
    marginTop: 0,
  },
  cropRow: {
    marginTop: 14,
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: 10,
  },
  flagLabelRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
    flexShrink: 1,
  },
  flagSwitch: {
    flexShrink: 0,
  },
  helpDot: {
    width: 18,
    height: 18,
    borderRadius: 9,
    borderWidth: 1.5,
    borderColor: '#111111',
    alignItems: 'center',
    justifyContent: 'center',
  },
  helpDotDark: {
    borderColor: '#FFFFFF',
  },
  helpMark: {
    color: '#111111',
    fontSize: 11,
    fontWeight: '800',
    marginTop: -0.5,
  },
  helpMarkDark: {
    color: '#FFFFFF',
  },
  cropLabel: {
    fontSize: 13,
    fontWeight: '700',
    color: '#111111',
  },
  switchTrack: {
    width: 44,
    height: 26,
    borderRadius: 13,
    backgroundColor: '#E4E4E4',
    justifyContent: 'center',
    paddingHorizontal: 2,
  },
  switchOn: {
    backgroundColor: '#FFD600',
  },
  switchThumb: {
    width: 22,
    height: 22,
    borderRadius: 11,
    backgroundColor: '#FFFFFF',
    alignSelf: 'flex-start',
  },
  switchThumbOn: {
    alignSelf: 'flex-end',
  },
  iconBtn: {
    marginTop: 22,
    width: 44,
    height: 44,
    borderRadius: 16,
    alignItems: 'center',
    justifyContent: 'center',
  },
  pillPink: {
    backgroundColor: '#FF4FA3',
  },
  pillPurple: {
    backgroundColor: '#7C4DFF',
  },
  plusH: {
    position: 'absolute',
    width: 16,
    height: 3,
    borderRadius: 2,
    backgroundColor: '#FFFFFF',
  },
  plusV: {
    position: 'absolute',
    width: 3,
    height: 16,
    borderRadius: 2,
    backgroundColor: '#FFFFFF',
  },
  play: {
    width: 0,
    height: 0,
    marginLeft: 3,
    borderTopWidth: 8,
    borderBottomWidth: 8,
    borderLeftWidth: 13,
    borderTopColor: 'transparent',
    borderBottomColor: 'transparent',
    borderLeftColor: '#FFFFFF',
  },
  empty: {
    marginTop: 22,
    fontSize: 15,
    color: '#6E6E6E',
  },
  pageSize: {
    marginTop: 18,
    fontSize: 14,
    color: '#6E6E6E',
  },
  pageGrid: {
    marginTop: 16,
    flexDirection: 'row',
    flexWrap: 'wrap',
  },
  gridCell: {
    width: '33.333%',
    aspectRatio: 1,
    padding: 4,
  },
  gridCellDragging: {
    opacity: 0.35,
  },
  dragGhost: {
    position: 'absolute',
    zIndex: 40,
    borderRadius: 16,
    overflow: 'hidden',
    backgroundColor: '#FFFFFF',
    transform: [{ scale: 1.06 }],
    shadowColor: '#111111',
    shadowOpacity: 0.22,
    shadowRadius: 12,
    shadowOffset: { width: 0, height: 8 },
    elevation: 10,
  },
  dragGhostImage: {
    width: '100%',
    height: '100%',
  },
  gridThumb: {
    flex: 1,
    borderRadius: 16,
    overflow: 'hidden',
    backgroundColor: '#FFFFFF',
  },
  gridImage: {
    width: '100%',
    height: '100%',
  },
  gridIndex: {
    position: 'absolute',
    left: 8,
    bottom: 8,
    minWidth: 22,
    height: 22,
    borderRadius: 11,
    paddingHorizontal: 6,
    backgroundColor: 'rgba(17, 17, 17, 0.72)',
    alignItems: 'center',
    justifyContent: 'center',
  },
  gridIndexLabel: {
    color: '#FFFFFF',
    fontSize: 12,
    fontWeight: '700',
  },
  gridDoneDot: {
    position: 'absolute',
    left: 10,
    top: 10,
    width: 12,
    height: 12,
    borderRadius: 6,
    backgroundColor: '#22C55E',
    borderWidth: 2,
    borderColor: '#FFFFFF',
  },
  gridPending: {
    position: 'absolute',
    left: 8,
    top: 8,
    borderRadius: 10,
    paddingHorizontal: 6,
    paddingVertical: 2,
    backgroundColor: '#FFD600',
  },
  gridPendingLabel: {
    color: '#111111',
    fontSize: 11,
    fontWeight: '800',
  },
  gridRemove: {
    position: 'absolute',
    top: 8,
    right: 8,
    width: 22,
    height: 22,
    borderRadius: 11,
    backgroundColor: '#111111',
    alignItems: 'center',
    justifyContent: 'center',
  },
  gridRemoveLabel: {
    color: '#FFFFFF',
    fontSize: 16,
    fontWeight: '700',
    marginTop: -1,
  },
  errorBanner: {
    marginTop: 58,
    marginHorizontal: 20,
    backgroundColor: '#111111',
    borderRadius: 18,
    paddingLeft: 16,
    paddingRight: 12,
    paddingVertical: 12,
    zIndex: 2,
    flexDirection: 'row',
    alignItems: 'flex-start',
    gap: 8,
  },
  errorBannerCopy: {
    flex: 1,
  },
  errorBannerClose: {
    color: '#FFFFFF',
    fontSize: 22,
    fontWeight: '700',
    lineHeight: 24,
  },
  errorBannerText: {
    color: '#FFD600',
    fontSize: 15,
    fontWeight: '700',
    lineHeight: 21,
  },
  errorBannerJob: {
    marginTop: 6,
    color: '#FFFFFF',
    fontSize: 13,
    fontVariant: ['tabular-nums'],
  },
  statusRow: {
    marginTop: 18,
    flexDirection: 'row',
    alignItems: 'center',
    gap: 10,
  },
  status: {
    color: '#111111',
    fontSize: 16,
    fontWeight: '700',
  },
  jobId: {
    marginTop: 8,
    color: '#888888',
    fontSize: 13,
    fontVariant: ['tabular-nums'],
  },
  resultCard: {
    backgroundColor: '#111111',
    borderRadius: 28,
    paddingHorizontal: 22,
    paddingVertical: 20,
  },
  resultTitle: {
    color: '#FFFFFF',
    fontSize: 28,
    fontWeight: '800',
    textAlign: 'center',
  },
  resultHint: {
    marginTop: 8,
    color: '#FFD600',
    fontSize: 15,
    fontWeight: '700',
    textAlign: 'center',
  },
  resultPrimary: {
    marginTop: 16,
    backgroundColor: '#FFD600',
    borderRadius: 22,
    paddingVertical: 14,
    alignItems: 'center',
  },
  resultPrimaryLabel: {
    color: '#111111',
    fontWeight: '800',
    fontSize: 18,
  },
  resultCropRow: {
    marginTop: 14,
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: 10,
    paddingHorizontal: 12,
  },
  resultCropLabel: {
    fontSize: 13,
    fontWeight: '700',
    color: '#FFFFFF',
  },
  resultRow: {
    marginTop: 12,
    flexDirection: 'row',
    gap: 12,
  },
  resultSecondary: {
    flex: 1,
    backgroundColor: '#FFFFFF',
    borderRadius: 22,
    paddingVertical: 16,
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 8,
  },
  resultSecondaryLabel: {
    color: '#111111',
    fontSize: 16,
    fontWeight: '700',
  },
  addIcon: {
    width: 16,
    height: 16,
    borderRadius: 8,
    borderWidth: 1.8,
    borderColor: '#111111',
    alignItems: 'center',
    justifyContent: 'center',
  },
  addIconH: {
    position: 'absolute',
    width: 8,
    height: 1.8,
    borderRadius: 1,
    backgroundColor: '#111111',
  },
  addIconV: {
    position: 'absolute',
    width: 1.8,
    height: 8,
    borderRadius: 1,
    backgroundColor: '#111111',
  },
  againIcon: {
    width: 16,
    height: 16,
  },
  againArc: {
    position: 'absolute',
    top: 1,
    left: 1,
    width: 14,
    height: 14,
    borderRadius: 7,
    borderWidth: 1.8,
    borderTopColor: 'transparent',
  },
  againHead: {
    position: 'absolute',
    top: 0,
    right: 1,
    width: 0,
    height: 0,
    borderLeftWidth: 3.5,
    borderRightWidth: 3.5,
    borderBottomWidth: 5,
    borderLeftColor: 'transparent',
    borderRightColor: 'transparent',
    transform: [{ rotate: '45deg' }],
  },
  resultAgain: {
    marginTop: 4,
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 6,
    paddingVertical: 10,
  },
  resultAgainLabel: {
    color: '#6E6E6E',
    fontSize: 15,
    fontWeight: '700',
  },
  mePage: {
    gap: 14,
  },
  meProfile: {
    backgroundColor: '#FFFFFF',
    borderRadius: 24,
    padding: 18,
    flexDirection: 'row',
    alignItems: 'center',
    gap: 14,
  },
  meAvatarWrap: {
    width: 56,
    height: 56,
  },
  meAvatar: {
    width: 56,
    height: 56,
    borderRadius: 28,
    backgroundColor: '#111111',
    alignItems: 'center',
    justifyContent: 'center',
  },
  meProviderBadge: {
    position: 'absolute',
    right: -3,
    bottom: -3,
    width: 22,
    height: 22,
    borderRadius: 11,
    backgroundColor: '#FFFFFF',
    borderWidth: 1.5,
    borderColor: '#FFFFFF',
    alignItems: 'center',
    justifyContent: 'center',
    shadowColor: '#111111',
    shadowOpacity: 0.12,
    shadowRadius: 3,
    shadowOffset: { width: 0, height: 1 },
    elevation: 2,
  },
  meAppleBadge: {
    backgroundColor: '#111111',
    borderColor: '#FFFFFF',
  },
  meAppleMark: {
    color: '#FFFFFF',
    fontSize: 12,
    fontWeight: '700',
    marginTop: -1,
  },
  googleGWrap: {
    width: 14,
    height: 14,
  },
  googleGRing: {
    width: 14,
    height: 14,
    borderRadius: 7,
    borderWidth: 3,
    borderTopColor: '#EA4335',
    borderRightColor: '#4285F4',
    borderBottomColor: '#34A853',
    borderLeftColor: '#FBBC05',
  },
  googleGGap: {
    position: 'absolute',
    top: 0,
    right: 0,
    width: 8,
    height: 7,
    backgroundColor: '#FFFFFF',
  },
  googleGBar: {
    position: 'absolute',
    top: 5.5,
    right: 0,
    width: 8,
    height: 3,
    backgroundColor: '#4285F4',
    borderTopRightRadius: 1,
    borderBottomRightRadius: 1,
  },
  meFace: {
    width: 32,
    height: 32,
    borderRadius: 16,
    backgroundColor: '#FFD600',
    flexDirection: 'row',
    justifyContent: 'space-evenly',
    alignItems: 'center',
    paddingTop: 6,
  },
  meSmile: {
    width: 12,
    height: 6,
    borderBottomLeftRadius: 8,
    borderBottomRightRadius: 8,
    borderWidth: 2,
    borderTopWidth: 0,
    borderColor: '#111111',
    marginTop: -8,
  },
  meProfileCopy: {
    flex: 1,
  },
  meName: {
    fontSize: 20,
    fontWeight: '800',
    color: '#111111',
  },
  meEmail: {
    marginTop: 4,
    fontSize: 14,
    color: '#6E6E6E',
  },
  meBadge: {
    marginTop: 8,
    alignSelf: 'flex-start',
    backgroundColor: '#FFD600',
    borderRadius: 10,
    paddingHorizontal: 8,
    paddingVertical: 3,
  },
  meBadgeLabel: {
    color: '#111111',
    fontSize: 12,
    fontWeight: '800',
  },
  meTable: {
    backgroundColor: '#FFFFFF',
    borderRadius: 24,
    paddingHorizontal: 18,
  },
  meRow: {
    minHeight: 58,
    flexDirection: 'row',
    alignItems: 'center',
  },
  meRowLabel: {
    fontSize: 16,
    fontWeight: '700',
    color: '#111111',
  },
  meRefreshWrap: {
    marginLeft: 8,
    width: 22,
    height: 22,
    alignItems: 'center',
    justifyContent: 'center',
  },
  meRefresh: {
    width: 14,
    height: 14,
  },
  meRefreshArc: {
    width: 12,
    height: 12,
    borderRadius: 6,
    borderWidth: 2,
    borderColor: '#6E6E6E',
    borderTopColor: 'transparent',
    transform: [{ rotate: '20deg' }],
  },
  meRefreshArrow: {
    position: 'absolute',
    top: 0,
    right: 1,
    width: 0,
    height: 0,
    borderLeftWidth: 3.5,
    borderRightWidth: 3.5,
    borderBottomWidth: 5,
    borderLeftColor: 'transparent',
    borderRightColor: 'transparent',
    borderBottomColor: '#6E6E6E',
  },
  meRowValue: {
    marginLeft: 'auto',
    flexDirection: 'row',
    alignItems: 'baseline',
    gap: 4,
  },
  meCount: {
    fontSize: 28,
    fontWeight: '800',
    color: '#111111',
  },
  meCountUnit: {
    fontSize: 15,
    fontWeight: '700',
    color: '#111111',
  },
  meChevron: {
    marginLeft: 4,
    fontSize: 22,
    color: '#C8C8C8',
    fontWeight: '500',
  },
  meDivider: {
    height: 1,
    backgroundColor: '#EFEFEF',
  },
  meCopy: {
    marginTop: 4,
    fontSize: 14,
    lineHeight: 20,
    color: '#8A8A8A',
  },
  homeChip: {
    marginTop: 20,
    alignSelf: 'flex-start',
    backgroundColor: '#FFD600',
    borderRadius: 18,
    paddingHorizontal: 16,
    paddingVertical: 8,
  },
  homeChipLabel: {
    color: '#111111',
    fontWeight: '800',
  },
  confirmDismiss: {
    flex: 1,
  },
  helpScrim: {
    flex: 1,
    justifyContent: 'center',
    backgroundColor: 'rgba(17, 17, 17, 0.45)',
    paddingHorizontal: 28,
  },
  helpCard: {
    backgroundColor: '#FFFFFF',
    borderRadius: 28,
    paddingHorizontal: 22,
    paddingTop: 24,
    paddingBottom: 20,
  },
  helpTitle: {
    color: '#111111',
    fontSize: 24,
    fontWeight: '800',
    textAlign: 'center',
  },
  helpCopy: {
    marginTop: 10,
    color: '#5A5A5A',
    fontSize: 15,
    fontWeight: '600',
    textAlign: 'center',
    lineHeight: 22,
  },
  helpClose: {
    marginTop: 20,
    backgroundColor: '#FFD600',
    borderRadius: 18,
    paddingVertical: 14,
    alignItems: 'center',
  },
  helpActions: {
    marginTop: 20,
    flexDirection: 'row',
    gap: 12,
  },
  helpCancel: {
    flex: 1,
    backgroundColor: '#F4F4F4',
    borderRadius: 18,
    paddingVertical: 14,
    alignItems: 'center',
  },
  helpCancelLabel: {
    color: '#111111',
    fontSize: 16,
    fontWeight: '800',
  },
  helpAction: {
    flex: 1,
    backgroundColor: '#FFD600',
    borderRadius: 18,
    paddingVertical: 14,
    alignItems: 'center',
  },
  confirmOkLabel: {
    color: '#111111',
    fontSize: 16,
    fontWeight: '800',
  },
  aboutScrim: {
    flex: 1,
    justifyContent: 'flex-end',
    backgroundColor: 'rgba(17, 17, 17, 0.45)',
  },
  aboutDismiss: {
    flex: 1,
  },
  aboutPanel: {
    backgroundColor: '#FFF7EC',
    borderTopLeftRadius: 28,
    borderTopRightRadius: 28,
    paddingHorizontal: 22,
    paddingTop: 10,
    paddingBottom: 28,
    maxHeight: '78%',
  },
  aboutHandle: {
    alignSelf: 'center',
    width: 40,
    height: 4,
    borderRadius: 2,
    backgroundColor: '#DDDDDD',
    marginBottom: 16,
  },
  aboutTitle: {
    fontSize: 24,
    fontWeight: '800',
    color: '#111111',
    marginBottom: 16,
  },
  aboutList: {
    flexGrow: 0,
  },
  aboutRow: {
    flexDirection: 'row',
    alignItems: 'flex-start',
    gap: 12,
    marginBottom: 14,
  },
  aboutDot: {
    width: 8,
    height: 8,
    borderRadius: 4,
    backgroundColor: '#FFD600',
    marginTop: 7,
  },
  aboutItem: {
    flex: 1,
    fontSize: 16,
    lineHeight: 24,
    color: '#333333',
  },
  aboutClose: {
    marginTop: 8,
    backgroundColor: '#FFD600',
    borderRadius: 18,
    paddingVertical: 14,
    alignItems: 'center',
  },
  aboutCloseLabel: {
    color: '#111111',
    fontSize: 16,
    fontWeight: '800',
  },
  tabDock: {
    position: 'absolute',
    left: 0,
    right: 0,
    bottom: 0,
    height: 92,
    overflow: 'visible',
  },
  tabBar: {
    height: 92,
    backgroundColor: '#FFFFFF',
    flexDirection: 'row',
    alignItems: 'flex-end',
    justifyContent: 'space-between',
    paddingHorizontal: 36,
    paddingBottom: 18,
  },
  tabItem: {
    width: 72,
    alignItems: 'center',
    gap: 6,
    paddingBottom: 4,
  },
  tabLabel: {
    fontSize: 13,
    color: '#8A8A8A',
    fontWeight: '600',
  },
  tabOn: {
    color: '#111111',
    fontWeight: '800',
  },
  homeGrid: {
    width: 22,
    height: 22,
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: 4,
  },
  homeDot: {
    width: 9,
    height: 9,
    borderRadius: 3,
  },
  tabIconOn: {
    backgroundColor: '#111111',
  },
  tabIconOff: {
    backgroundColor: '#8A8A8A',
  },
  personIcon: {
    width: 22,
    height: 22,
    alignItems: 'center',
  },
  personHead: {
    width: 10,
    height: 10,
    borderRadius: 5,
    borderWidth: 2,
  },
  personBody: {
    marginTop: 2,
    width: 18,
    height: 10,
    borderTopLeftRadius: 10,
    borderTopRightRadius: 10,
    borderWidth: 2,
    borderBottomWidth: 0,
  },
  tabIconOnBorder: {
    borderColor: '#111111',
  },
  tabIconOffBorder: {
    borderColor: '#8A8A8A',
  },
  tabCamSlot: {
    width: 72,
  },
  tabCamWrap: {
    position: 'absolute',
    top: -16,
    alignSelf: 'center',
    zIndex: 2,
  },
  tabCam: {
    width: 72,
    height: 72,
    borderRadius: 36,
    backgroundColor: '#FFD600',
    alignItems: 'center',
    justifyContent: 'center',
  },
  tabCamGlyph: {
    alignItems: 'center',
  },
  tabCamFlash: {
    width: 16,
    borderBottomWidth: 7,
    borderBottomColor: '#111111',
    borderLeftWidth: 5,
    borderRightWidth: 5,
    borderLeftColor: 'transparent',
    borderRightColor: 'transparent',
    marginBottom: -1,
  },
  tabCamBody: {
    width: 30,
    height: 20,
    borderRadius: 6,
    backgroundColor: '#111111',
    alignItems: 'center',
    justifyContent: 'center',
  },
  tabCamLens: {
    width: 11,
    height: 11,
    borderRadius: 6,
    borderWidth: 2.5,
    borderColor: '#FFD600',
  },
  viewer: {
    flex: 1,
    backgroundColor: '#111111',
    paddingTop: 56,
  },
  viewerHeader: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingHorizontal: 16,
    paddingBottom: 12,
  },
  backButton: {
    minWidth: 64,
    paddingVertical: 8,
  },
  backLabel: {
    color: '#FFFFFF',
    fontSize: 16,
    fontWeight: '600',
  },
  backSpacer: {
    minWidth: 64,
  },
  viewerTitle: {
    color: '#FFFFFF',
    fontSize: 16,
    fontWeight: '700',
  },
  viewerCompare: {
    backgroundColor: '#FFD600',
    borderRadius: 14,
    paddingHorizontal: 10,
    paddingVertical: 8,
    alignItems: 'center',
  },
  viewerCompareLabel: {
    color: '#111111',
    fontSize: 13,
    fontWeight: '800',
  },
  viewerMode: {
    alignItems: 'center',
    paddingBottom: 8,
  },
  viewerModeLabel: {
    color: '#FFD600',
    fontSize: 13,
    fontWeight: '800',
  },
  viewerPager: {
    flex: 1,
  },
  viewerStage: {
    flex: 1,
    backgroundColor: '#111111',
    overflow: 'hidden',
  },
  viewerZoom: {
    flex: 1,
  },
  thumbStrip: {
    maxHeight: 102,
    backgroundColor: '#111111',
  },
  thumbStripContent: {
    paddingHorizontal: 16,
    paddingTop: 10,
    paddingBottom: 28,
    alignItems: 'center',
  },
  thumb: {
    width: THUMB_WIDTH,
    height: 74,
    borderRadius: 10,
    overflow: 'hidden',
    marginRight: THUMB_GAP,
    borderWidth: 2,
    borderColor: 'transparent',
    backgroundColor: '#2A2A2A',
  },
  thumbOn: {
    borderColor: '#FFD600',
  },
  thumbImage: {
    width: '100%',
    height: '100%',
  },
  thumbIndex: {
    position: 'absolute',
    left: 4,
    bottom: 4,
    minWidth: 18,
    height: 18,
    paddingHorizontal: 4,
    borderRadius: 9,
    backgroundColor: 'rgba(17, 17, 17, 0.72)',
    alignItems: 'center',
    justifyContent: 'center',
  },
  thumbIndexLabel: {
    color: '#FFFFFF',
    fontSize: 11,
    fontWeight: '700',
  },
});
