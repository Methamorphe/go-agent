import { describe, expect, it } from "vitest";
import { virtualRange } from "./virtual-feed";

describe("virtualRange", () => {
  it("keeps the DOM window bounded for a 100k logical feed", () => {
    expect(virtualRange(100_000, 100_000, 260)).toEqual({ start: 99_740, end: 100_000 });
  });

  it("clamps an invalid end safely", () => {
    expect(virtualRange(10, 200, 4)).toEqual({ start: 6, end: 10 });
  });
});
