/** 图在私密相册内时不能改回 public（与后端 ErrAlbumForcesPrivate 对齐）。 */
export function albumForcesPrivate(
  albums: { id: number; visibility: string }[] | undefined,
  albumId: number | null | undefined,
): boolean {
  if (albumId == null || albumId === 0 || !albums) return false
  return albums.some((a) => a.id === albumId && a.visibility === 'private')
}
