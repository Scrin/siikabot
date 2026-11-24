// A real response of DELETE /api/memories, saved by the end-to-end test TestAPIContract. tsc checks that DeleteAllMemoriesResponse fits it,
// and the test that the API still answers in its shape. Don't edit it by hand: run the test with
// SIIKABOT_E2E_UPDATE_FIXTURES=1 to save it again.
import type { DeleteAllMemoriesResponse } from '../types'

export const memoriesDeleted: DeleteAllMemoriesResponse = {
  "deleted_count": 2
}
