import { API_BASE, httpClient } from './helpers'
import { ApiError } from './http-client'

export interface UserProfile {
  id: string
  email: string
  /** Shown in the account menu; new accounts start as "Admin". */
  display_name: string
  /** Data URL of the uploaded image; empty when none is set. */
  avatar: string
}

export const userApi = {
  async getProfile(): Promise<UserProfile> {
    const result = await httpClient.get<UserProfile>(`${API_BASE}/user/profile`)
    if (!result.success) throw new Error('Failed to load profile')
    return result.data!
  },

  /** Uploads an image; returns the avatar as the server redrew and stored it. */
  async updateAvatar(image: Blob): Promise<string> {
    const form = new FormData()
    form.append('avatar', image, 'avatar.png')
    // A non-JSON type keeps axios from serialising the form; the browser then
    // sets the multipart boundary itself.
    const result = await httpClient.put<{ avatar: string }>(
      `${API_BASE}/user/avatar`,
      form,
      { 'Content-Type': 'multipart/form-data' }
    )
    if (!result.success)
      throw new Error(result.message || 'Failed to update avatar')
    return result.data!.avatar
  },

  /** Returns the name as the server stored it (trimmed). */
  async updateDisplayName(name: string): Promise<string> {
    const result = await httpClient.put<{ display_name: string }>(
      `${API_BASE}/user/display-name`,
      { display_name: name }
    )
    if (!result.success)
      throw new Error(result.message || 'Failed to update display name')
    return result.data!.display_name
  },

  async removeAvatar(): Promise<void> {
    const result = await httpClient.delete(`${API_BASE}/user/avatar`)
    if (!result.success)
      throw new Error(result.message || 'Failed to remove avatar')
  },

  /** Throws an ApiError with errorKey "current_password_incorrect" when the current password does not match. */
  async changePassword(
    currentPassword: string,
    newPassword: string
  ): Promise<void> {
    const result = await httpClient.put(`${API_BASE}/user/password`, {
      current_password: currentPassword,
      new_password: newPassword,
    })
    if (!result.success)
      throw new ApiError(
        result.message || 'Failed to update password',
        result.errorKey,
        undefined,
        result.statusCode
      )
  },
}
