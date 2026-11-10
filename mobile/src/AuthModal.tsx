import { useEffect, useState } from 'react';
import { Modal, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';
import { StatusBar } from 'expo-status-bar';

import { loginGoogle } from './api';
import { isGoogleCancel, signInWithGoogle } from './googleAuth';
import { setAccessToken } from './session';
import { copy, LOCALES, type Locale } from './i18n';

export default function AuthModal({
  visible,
  locale,
  onChangeLocale,
  onClose,
  onSignedIn,
}: {
  visible: boolean;
  locale: Locale;
  onChangeLocale: (locale: Locale) => void;
  onClose: () => void;
  onSignedIn: (account?: {
    name: string;
    email: string;
    remaining: number;
    provider: 'google' | 'apple';
  }) => void;
}) {
  const t = copy[locale];
  const [googleBusy, setGoogleBusy] = useState(false);
  const [authError, setAuthError] = useState<string | null>(null);

  useEffect(() => {
    if (!visible) {
      return;
    }
    setAuthError(null);
    setGoogleBusy(false);
  }, [visible]);

  async function onGoogle() {
    if (googleBusy) {
      return;
    }
    setAuthError(null);
    setGoogleBusy(true);
    try {
      const profile = await signInWithGoogle();
      if (profile == null) {
        return;
      }
      const account = await loginGoogle(profile.idToken);
      setAccessToken(account.token);
      onSignedIn({
        name: account.name,
        email: account.email,
        remaining: account.remaining,
        provider: 'google',
      });
    } catch (error) {
      if (!isGoogleCancel(error)) {
        console.error('Google sign-in failed', error);
        setAuthError(t.googleFailed);
      }
    } finally {
      setGoogleBusy(false);
    }
  }

  return (
    <Modal visible={visible} animationType="slide" presentationStyle="fullScreen" onRequestClose={onClose}>
      <View style={styles.screen}>
        <StatusBar style="dark" />
        <View style={styles.blobYellow} />
        <View style={styles.blobPink} />
        <ScrollView contentContainerStyle={styles.body} showsVerticalScrollIndicator={false}>
          <View style={styles.topRow}>
            <Pressable onPress={onClose} hitSlop={8}>
              <Text style={styles.topLink}>{t.close}</Text>
            </Pressable>
            <View style={styles.langRow}>
              {LOCALES.map((item) => {
                const on = item.id === locale;
                return (
                  <Pressable
                    key={item.id}
                    style={[styles.langChip, on ? styles.langChipOn : null]}
                    onPress={() => onChangeLocale(item.id)}
                    hitSlop={6}
                  >
                    <Text style={[styles.langChipLabel, on ? styles.langChipLabelOn : null]}>{item.label}</Text>
                  </Pressable>
                );
              })}
            </View>
          </View>

          <View style={styles.brand}>
            <View style={styles.logo}>
              <View style={styles.logoFace}>
                <View style={styles.logoEye} />
                <View style={styles.logoEye} />
              </View>
              <View style={styles.logoSmile} />
            </View>
            <Text style={styles.title}>空白卷</Text>
            <Text style={styles.subtitle}>{t.subtitle}</Text>
          </View>

          <View style={styles.card}>
            <Pressable style={styles.apple}>
              <Text style={styles.appleMark}></Text>
              <Text style={styles.appleLabel}>{t.appleLogin}</Text>
            </Pressable>
            <Pressable
              style={[styles.google, googleBusy ? styles.googleBusy : null]}
              disabled={googleBusy}
              onPress={() => void onGoogle()}
            >
              <Text style={styles.googleMark}>G</Text>
              <Text style={styles.googleLabel}>{t.googleLogin}</Text>
            </Pressable>
            {authError != null ? <Text style={styles.authError}>{authError}</Text> : null}
          </View>
        </ScrollView>
      </View>
    </Modal>
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
    left: -40,
    width: 260,
    height: 260,
    borderRadius: 130,
    backgroundColor: '#FFD600',
    opacity: 0.35,
  },
  blobPink: {
    position: 'absolute',
    top: -20,
    right: -70,
    width: 220,
    height: 220,
    borderRadius: 110,
    backgroundColor: '#FF7A59',
    opacity: 0.22,
  },
  body: {
    paddingTop: 58,
    paddingHorizontal: 20,
    paddingBottom: 40,
  },
  topRow: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    marginBottom: 18,
  },
  topLink: {
    fontSize: 16,
    fontWeight: '700',
    color: '#111111',
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
  brand: {
    alignItems: 'center',
    marginBottom: 22,
  },
  logo: {
    width: 64,
    height: 64,
    borderRadius: 18,
    backgroundColor: '#111111',
    alignItems: 'center',
    justifyContent: 'center',
  },
  logoFace: {
    width: 36,
    height: 36,
    borderRadius: 18,
    backgroundColor: '#FFD600',
    flexDirection: 'row',
    justifyContent: 'space-evenly',
    alignItems: 'center',
    paddingTop: 6,
  },
  logoEye: {
    width: 5,
    height: 5,
    borderRadius: 2.5,
    backgroundColor: '#111111',
  },
  logoSmile: {
    width: 14,
    height: 7,
    borderBottomLeftRadius: 10,
    borderBottomRightRadius: 10,
    borderWidth: 2.5,
    borderTopWidth: 0,
    borderColor: '#111111',
    marginTop: -10,
  },
  title: {
    marginTop: 14,
    fontSize: 32,
    fontWeight: '800',
    color: '#111111',
  },
  subtitle: {
    marginTop: 6,
    fontSize: 16,
    color: '#5A5A5A',
  },
  card: {
    backgroundColor: '#FFFFFF',
    borderRadius: 28,
    padding: 18,
  },
  apple: {
    backgroundColor: '#111111',
    borderRadius: 18,
    paddingVertical: 14,
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 8,
  },
  appleMark: {
    color: '#FFFFFF',
    fontSize: 20,
    marginTop: -2,
  },
  appleLabel: {
    color: '#FFFFFF',
    fontSize: 16,
    fontWeight: '800',
  },
  google: {
    marginTop: 10,
    backgroundColor: '#FFFFFF',
    borderRadius: 18,
    paddingVertical: 13,
    borderWidth: 1,
    borderColor: '#E4E4E4',
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 8,
  },
  googleMark: {
    color: '#EA4335',
    fontSize: 18,
    fontWeight: '800',
  },
  googleLabel: {
    color: '#111111',
    fontSize: 16,
    fontWeight: '800',
  },
  googleBusy: {
    opacity: 0.55,
  },
  authError: {
    marginTop: 12,
    color: '#D81B60',
    fontSize: 14,
    fontWeight: '700',
    textAlign: 'center',
  },
});
