import useSWR from 'swr'
import { api } from '@/api'
import { useAuth } from '@/contexts/auth-context'

/** The signed-in user's profile; every caller shares one cached copy. */
export function useProfile() {
  const { user, token } = useAuth()
  return useSWR(
    user && token ? ['user-profile', user.id] : null,
    () => api.getProfile(),
    { revalidateOnFocus: false }
  )
}
