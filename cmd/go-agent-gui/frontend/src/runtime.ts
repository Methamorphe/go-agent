import { DesktopService } from "../bindings/github.com/Methamorphe/go-agent/internal/gui";
import type {
  Bootstrap,
  ConversationViewport,
  LiveSnapshot,
  Refresh,
  RuntimeClient,
  SearchResult,
  Snapshot,
} from "./types";

export class WailsRuntimeClient implements RuntimeClient {
  bootstrap(): Promise<Bootstrap> {
    return DesktopService.Bootstrap() as Promise<Bootstrap>;
  }

  attach(request: Parameters<RuntimeClient["attach"]>[0]): Promise<Snapshot> {
    return DesktopService.Attach(request as never) as Promise<Snapshot>;
  }

  refresh(request: Parameters<RuntimeClient["refresh"]>[0]): Promise<Refresh> {
    return DesktopService.Refresh(request as never) as Promise<Refresh>;
  }

  history(request: Parameters<RuntimeClient["history"]>[0]): Promise<ConversationViewport> {
    return DesktopService.History(request as never) as Promise<ConversationViewport>;
  }

  search(request: Parameters<RuntimeClient["search"]>[0]): Promise<SearchResult> {
    return DesktopService.Search(request as never) as Promise<SearchResult>;
  }

  live(agentID: string): Promise<LiveSnapshot> {
    return DesktopService.Live(agentID as never) as Promise<LiveSnapshot>;
  }

  sendMessage(request: Parameters<RuntimeClient["sendMessage"]>[0]): ReturnType<RuntimeClient["sendMessage"]> {
    return DesktopService.SendMessage(request as never) as ReturnType<RuntimeClient["sendMessage"]>;
  }

  suspend(request: Parameters<RuntimeClient["suspend"]>[0]): Promise<unknown> {
    return DesktopService.Suspend(request as never) as Promise<unknown>;
  }

  resume(request: Parameters<RuntimeClient["resume"]>[0]): Promise<unknown> {
    return DesktopService.Resume(request as never) as Promise<unknown>;
  }

  operateTransaction(request: Parameters<RuntimeClient["operateTransaction"]>[0]): Promise<unknown> {
    return DesktopService.OperateTransaction(request as never) as Promise<unknown>;
  }
}

export function errorMessage(error: unknown): string {
  if (error instanceof Error) return error.message;
  if (typeof error === "string") return error;
  if (error && typeof error === "object") {
    const candidate = error as { message?: unknown; Message?: unknown };
    if (typeof candidate.message === "string") return candidate.message;
    if (typeof candidate.Message === "string") return candidate.Message;
  }
  return "Unknown runtime error";
}
