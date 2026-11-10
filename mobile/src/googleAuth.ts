import { Platform } from 'react-native';
import {
  GoogleSignin,
  isErrorWithCode,
  isSuccessResponse,
  statusCodes,
} from '@react-native-google-signin/google-signin';

import { GOOGLE_IOS_CLIENT_ID, GOOGLE_WEB_CLIENT_ID } from './config';

let configured = false;

function ensureConfigured() {
  if (configured) {
    return;
  }
  GoogleSignin.configure({
    iosClientId: GOOGLE_IOS_CLIENT_ID,
    webClientId: GOOGLE_WEB_CLIENT_ID,
  });
  configured = true;
}

export type GoogleProfile = {
  name: string;
  email: string;
  idToken: string;
};

export async function signInWithGoogle(): Promise<GoogleProfile | null> {
  ensureConfigured();
  if (Platform.OS === 'android') {
    await GoogleSignin.hasPlayServices({ showPlayServicesUpdateDialog: true });
  }
  const response = await GoogleSignin.signIn();
  if (!isSuccessResponse(response)) {
    return null;
  }
  const { name, email } = response.data.user;
  const idToken = response.data.idToken;
  if (idToken == null || idToken.length === 0) {
    throw new Error('missing Google id token');
  }
  return {
    name: name ?? email,
    email,
    idToken,
  };
}

export function isGoogleCancel(error: unknown): boolean {
  return isErrorWithCode(error) && error.code === statusCodes.SIGN_IN_CANCELLED;
}