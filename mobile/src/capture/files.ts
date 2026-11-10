import { File } from 'expo-file-system';

/**
 * Deletes a cached photo. Photos and crops are written to the app's cache on
 * every scan and nothing else removes them; a file that is already gone or
 * cannot be deleted is not worth failing a scan over.
 */
export function deleteQuietly(uri: string): void {
  try {
    new File(uri).delete();
  } catch {
    // already deleted, or not ours to delete
  }
}
