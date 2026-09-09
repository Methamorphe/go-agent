import { MAX_RENDERED_BLOCKS } from "./store";
import type { Block } from "./types";

export const ESTIMATED_BLOCK_HEIGHT = 94;

export function virtualRange(total: number, end: number, size = MAX_RENDERED_BLOCKS): { start: number; end: number } {
  const safeEnd = Math.max(0, Math.min(total, end));
  return { start: Math.max(0, safeEnd - Math.max(1, size)), end: safeEnd };
}

export class VirtualFeed {
  private blocks: Block[] = [];
  private end = 0;
  private pinnedToBottom = true;
  private renderItem: (block: Block) => string;

  constructor(
    private readonly scroller: HTMLElement,
    private readonly topSpacer: HTMLElement,
    private readonly content: HTMLElement,
    private readonly bottomSpacer: HTMLElement,
    renderItem: (block: Block) => string,
  ) {
    this.renderItem = renderItem;
    this.scroller.addEventListener("scroll", () => this.onScroll(), { passive: true });
  }

  setBlocks(blocks: Block[]): void {
    const previousLast = this.blocks.at(-1)?.id;
    const nextLast = blocks.at(-1)?.id;
    const changed = this.blocks.length !== blocks.length || previousLast !== nextLast;
    this.blocks = blocks;
    if (!changed) return;
    if (this.pinnedToBottom || this.end === 0) this.end = blocks.length;
    else this.end = Math.min(this.end, blocks.length);
    this.render();
    if (this.pinnedToBottom) requestAnimationFrame(() => this.scrollToBottom(false));
  }

  showLatest(): void {
    this.pinnedToBottom = true;
    this.end = this.blocks.length;
    this.render();
    requestAnimationFrame(() => this.scrollToBottom(true));
  }

  scrollToBlock(id: string): boolean {
    const index = this.blocks.findIndex((block) => block.id === id);
    if (index < 0) return false;
    this.pinnedToBottom = false;
    this.end = Math.min(this.blocks.length, index + Math.floor(MAX_RENDERED_BLOCKS / 2));
    if (this.end < MAX_RENDERED_BLOCKS) this.end = Math.min(this.blocks.length, MAX_RENDERED_BLOCKS);
    this.render();
    requestAnimationFrame(() => {
      this.content.querySelector<HTMLElement>(`[data-block-id="${CSS.escape(id)}"]`)?.scrollIntoView({ block: "center", behavior: "smooth" });
    });
    return true;
  }

  private onScroll(): void {
    const bottomDistance = this.scroller.scrollHeight - this.scroller.scrollTop - this.scroller.clientHeight;
    this.pinnedToBottom = bottomDistance < 90 && this.end >= this.blocks.length;
    const range = virtualRange(this.blocks.length, this.end);
    if (this.scroller.scrollTop < 140 && range.start > 0) {
      const previousStart = range.start;
      this.end = Math.max(MAX_RENDERED_BLOCKS, this.end - Math.floor(MAX_RENDERED_BLOCKS * 0.65));
      this.render();
      const nextStart = virtualRange(this.blocks.length, this.end).start;
      this.scroller.scrollTop += Math.max(0, previousStart - nextStart) * ESTIMATED_BLOCK_HEIGHT;
      return;
    }
    if (bottomDistance < 220 && this.end < this.blocks.length) {
      this.end = Math.min(this.blocks.length, this.end + Math.floor(MAX_RENDERED_BLOCKS * 0.65));
      this.render();
    }
  }

  private render(): void {
    const range = virtualRange(this.blocks.length, this.end || this.blocks.length);
    this.topSpacer.style.height = `${range.start * ESTIMATED_BLOCK_HEIGHT}px`;
    this.bottomSpacer.style.height = `${Math.max(0, this.blocks.length - range.end) * ESTIMATED_BLOCK_HEIGHT}px`;
    this.content.innerHTML = this.blocks.slice(range.start, range.end).map(this.renderItem).join("");
  }

  private scrollToBottom(smooth: boolean): void {
    this.scroller.scrollTo({ top: this.scroller.scrollHeight, behavior: smooth ? "smooth" : "auto" });
  }
}
