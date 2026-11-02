import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '@/api'
import { useAuth } from '@/contexts/auth-context'

/** The signed-in user's profile; every caller shares one cached copy. */
export function useProfile() {
  const { user, token } = useAuth()
  const client = useQueryClient()
  const queryKey = ['user-profile', user?.id]
  const query = useQuery({
    queryKey,
    queryFn: () => api.getProfile(),
    enabled: Boolean(user && token),
    refetchOnWindowFocus: false,
  })
  return {
    ...query,
    /** Shows what has just been saved, without reading it back. */
    setProfile: (profile: typeof query.data) =>
      client.setQueryData(queryKey, profile),
  }
}
