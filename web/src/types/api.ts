export interface ApiResponse<T> {
  data: T
  meta?: {
    total?: number
    page?: number
    per_page?: number
  }
}

export interface ApiErrorResponse {
  error: {
    code: string
    message: string
  }
}
