import { File } from 'expo-file-system';
import { EncodingType, writeAsStringAsync } from 'expo-file-system/legacy';

import { API_BASE_URL } from './config';
import { authHeaders, notifyUnauthorized } from './session';

export class UnauthorizedError extends Error {
  constructor() {
    super('unauthorized');
    this.name = 'UnauthorizedError';
  }
}

export function isUnauthorized(error: unknown): boolean {
  return error instanceof UnauthorizedError || (error instanceof Error && error.name === 'UnauthorizedError');
}

export const ApiCode = {
  OK: 200,
  AuthInvalid: 1001,
  AuthUnavailable: 1002,
  AuthFailed: 1003,
  Unauthorized: 1004,
  InvalidRequest: 1101,
  NoPages: 1102,
  InvalidImage: 1103,
  TooManyPages: 1104,
  ImageTooLarge: 1105,
  JobNotFound: 1201,
  JobNotReady: 1202,
  JobFailed: 1203,
  PageNotFound: 1204,
  PDFFailed: 1301,
  InsufficientCredits: 1401,
  Internal: 1901,
} as const;

export type ApiCode = (typeof ApiCode)[keyof typeof ApiCode];

export type JobStatus = 'pending' | 'running' | 'succeeded' | 'failed';

export type Account = {
  user_id: string;
  email: string;
  name: string;
  remaining: number;
  token: string;
};

export type Job = {
  id: string;
  status: JobStatus;
  page_count: number;
  error: string | null;
  remaining?: number;
  previews?: string[];
  pdf?: string;
};

type Envelope<T> = {
  code: number;
  reason: string;
  data: T | null;
};

export async function getMe(): Promise<Account> {
  const response = await fetch(`${API_BASE_URL}/me`, {
    headers: authHeaders(),
  });
  return readJson<Account>(response);
}

export async function loginGoogle(idToken: string): Promise<Account> {
  const response = await fetch(`${API_BASE_URL}/auth/google`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id_token: idToken }),
  });
  return readJson<Account>(response);
}

export async function createJob(
  uris: string[],
  crop = false,
): Promise<{ id: string; status: JobStatus; remaining: number }> {
  const body = new FormData();
  uris.forEach((uri, index) => {
    body.append('pages', new File(uri), `page-${index}.jpg`);
  });
  body.append('crop', crop ? '1' : '0');
  const response = await fetch(`${API_BASE_URL}/jobs`, {
    method: 'POST',
    headers: authHeaders(),
    body,
  });
  return readJson(response);
}

export async function getJob(id: string): Promise<Job> {
  const response = await fetch(`${API_BASE_URL}/jobs/${id}`, {
    headers: authHeaders(),
  });
  return readJson(response);
}

export async function createPdf(uris: string[], destination: string): Promise<void> {
  const body = new FormData();
  uris.forEach((uri, index) => {
    body.append('pages', new File(uri), `page-${index}.jpg`);
  });
  const response = await fetch(`${API_BASE_URL}/pdf`, {
    method: 'POST',
    headers: authHeaders(),
    body,
  });
  const contentType = response.headers.get('content-type') ?? '';
  if (contentType.includes('application/json')) {
    await readJson(response);
    throw new Error('request failed');
  }
  const bytes = new Uint8Array(await response.arrayBuffer());
  await writeAsStringAsync(destination, bytesToBase64(bytes), {
    encoding: EncodingType.Base64,
  });
}

function bytesToBase64(bytes: Uint8Array): string {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';
  let output = '';
  for (let i = 0; i < bytes.length; i += 3) {
    const a = bytes[i] ?? 0;
    const b = bytes[i + 1] ?? 0;
    const c = bytes[i + 2] ?? 0;
    const triple = (a << 16) | (b << 8) | c;
    output += chars[(triple >> 18) & 63];
    output += chars[(triple >> 12) & 63];
    output += i + 1 < bytes.length ? chars[(triple >> 6) & 63] : '=';
    output += i + 2 < bytes.length ? chars[triple & 63] : '=';
  }
  return output;
}

export function failFromEnvelope(payload: { code?: number; reason?: string }): never {
  if (payload.code === ApiCode.Unauthorized) {
    throwUnauthorized();
  }
  throw new Error(payload.reason || 'request failed');
}

async function readJson<T>(response: Response): Promise<T> {
  let payload: Envelope<T>;
  try {
    payload = (await response.json()) as Envelope<T>;
  } catch {
    throw new Error('request failed');
  }
  if (payload.code !== ApiCode.OK || payload.data == null) {
    failFromEnvelope(payload);
  }
  return payload.data;
}

function throwUnauthorized(): never {
  notifyUnauthorized();
  throw new UnauthorizedError();
}
