/** User-facing API errors with Portuguese messages. */

export class ApiError extends Error {
  readonly status: number;
  readonly path: string;
  readonly code?: string;

  constructor(status: number, path: string, message: string, code?: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.path = path;
    this.code = code;
  }
}

function messageForStatus(status: number, serverMessage?: string): string {
  if (serverMessage && serverMessage.trim().length > 0) {
    const lower = serverMessage.toLowerCase();
    // Prefer Portuguese server messages; map common English API strings.
    if (!/[áàâãéêíóôõúç]/i.test(serverMessage)) {
      if (lower.includes("list environments")) {
        return "Não foi possível listar os ambientes.";
      }
      if (lower.includes("required")) {
        return "Dados incompletos na requisição.";
      }
    } else {
      return serverMessage.trim();
    }
  }

  if (status === 0) {
    return "Não foi possível conectar à API. Confirme se o serviço está saudável e se o proxy do frontend encaminha /api para a API.";
  }
  if (status === 400) {
    return "Requisição inválida. Revise os filtros ou os dados enviados.";
  }
  if (status === 401 || status === 403) {
    return "Acesso não autorizado a este recurso.";
  }
  if (status === 404) {
    return "Recurso não encontrado. Ele pode ter sido removido ou o identificador está incorreto.";
  }
  if (status === 409) {
    return "Conflito: o recurso já existe ou está em uso.";
  }
  if (status === 422) {
    return "Dados inválidos. Revise o formulário e tente novamente.";
  }
  if (status === 429) {
    return "Muitas requisições. Aguarde um momento e tente de novo.";
  }
  if (status >= 500) {
    return "Erro interno do servidor. Tente novamente em instantes.";
  }
  return `Falha na API (código ${status}).`;
}

export async function toApiError(
  response: Response,
  path: string,
): Promise<ApiError> {
  let serverMessage = "";
  let code: string | undefined;
  try {
    const text = await response.text();
    if (text) {
      try {
        const json = JSON.parse(text) as {
          error?: string;
          message?: string;
          code?: string;
        };
        serverMessage = json.error || json.message || text;
        code = json.code;
      } catch {
        serverMessage = text.slice(0, 400);
      }
    }
  } catch {
    // ignore body parse failures
  }
  return new ApiError(
    response.status,
    path,
    messageForStatus(response.status, serverMessage),
    code,
  );
}

export function networkApiError(path: string, cause?: unknown): ApiError {
  if (cause instanceof DOMException && cause.name === "AbortError") {
    return new ApiError(0, path, "");
  }
  if (cause instanceof Error && cause.name === "AbortError") {
    return new ApiError(0, path, "");
  }
  const detail =
    cause instanceof Error && cause.message ? ` (${cause.message})` : "";
  return new ApiError(0, path, messageForStatus(0) + detail);
}

export function formatError(
  err: unknown,
  fallback = "Ocorreu um erro inesperado.",
): string {
  if (err instanceof DOMException && err.name === "AbortError") return "";
  if (err instanceof Error && err.name === "AbortError") return "";
  if (err instanceof ApiError) {
    return err.message;
  }
  if (err instanceof Error && err.message) {
    return err.message;
  }
  return fallback;
}
