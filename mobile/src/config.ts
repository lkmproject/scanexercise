export const API_BASE_URL = (
  process.env.EXPO_PUBLIC_API_BASE_URL ?? 'http://192.168.1.24:8000'
).replace(/\/$/, '');

export const MAX_PAGES = 20;

export const GOOGLE_IOS_CLIENT_ID =
  '430840613406-tnq189it2aso4uvbhb5vec8tss43js3i.apps.googleusercontent.com';

export const GOOGLE_IOS_URL_SCHEME =
  'com.googleusercontent.apps.430840613406-tnq189it2aso4uvbhb5vec8tss43js3i';

export const GOOGLE_WEB_CLIENT_ID =
  '430840613406-4eki8kbemjbhc0jbah93v1vdc33fuf6p.apps.googleusercontent.com';
