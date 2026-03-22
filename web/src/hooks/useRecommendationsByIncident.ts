import { useQuery } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'
import type { Recommendation } from '@/types/models'

export function useRecommendationsByIncident(incidentId: number | undefined) {
  return useQuery({
    queryKey: ['recommendations', 'incident', incidentId],
    queryFn: async () => {
      const res = await apiFetch(`/recommendations?incident_id=${incidentId}`)
      const json = await res.json()
      return json.data as Recommendation[]
    },
    enabled: !!incidentId && incidentId > 0,
  })
}
