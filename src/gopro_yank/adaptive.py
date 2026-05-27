"""Adaptive concurrency limiter.

A semaphore whose target capacity moves up on sustained success and down
on failure. Uses asyncio.Condition for waiters so the cap can be resized
without recreating the primitive.

Algorithm:
  - Start at `initial`. Floor `floor`, ceiling `ceiling`.
  - On every successful release: increment a streak counter.
    When streak >= grow_after, increase target by 1 and reset streak.
  - On a failure: reduce target to max(floor, target - shrink_step).
  - The cap is enforced inside acquire(): if inflight >= target, wait.

The primitive is safe to use concurrently across many tasks.
"""

from __future__ import annotations

import asyncio
from dataclasses import dataclass


@dataclass(slots=True)
class LimiterStats:
    target: int
    inflight: int
    successes: int
    failures: int
    streak: int


class AdaptiveLimiter:
    def __init__(
        self,
        *,
        initial: int = 4,
        floor: int = 1,
        ceiling: int = 20,
        grow_after: int = 8,
        shrink_step: int = 1,
    ) -> None:
        if not floor <= initial <= ceiling:
            raise ValueError("must have floor <= initial <= ceiling")
        self._target = initial
        self._floor = floor
        self._ceiling = ceiling
        self._grow_after = grow_after
        self._shrink_step = shrink_step
        self._inflight = 0
        self._streak = 0
        self._successes = 0
        self._failures = 0
        self._cond = asyncio.Condition()

    async def acquire(self) -> None:
        async with self._cond:
            while self._inflight >= self._target:
                await self._cond.wait()
            self._inflight += 1

    async def release(self, *, success: bool) -> None:
        async with self._cond:
            self._inflight -= 1
            if success:
                self._successes += 1
                self._streak += 1
                if self._streak >= self._grow_after and self._target < self._ceiling:
                    self._target += 1
                    self._streak = 0
                    self._cond.notify()  # one waiter can now proceed
            else:
                self._failures += 1
                self._streak = 0
                new_target = max(self._floor, self._target - self._shrink_step)
                if new_target != self._target:
                    self._target = new_target
                    # nothing to notify; cap shrunk
            self._cond.notify()

    def stats(self) -> LimiterStats:
        return LimiterStats(
            target=self._target,
            inflight=self._inflight,
            successes=self._successes,
            failures=self._failures,
            streak=self._streak,
        )

    class _CM:
        __slots__ = ("limiter", "success")

        def __init__(self, limiter: AdaptiveLimiter) -> None:
            self.limiter = limiter
            self.success = False  # set by caller before exit

        async def __aenter__(self) -> AdaptiveLimiter._CM:
            await self.limiter.acquire()
            return self

        async def __aexit__(self, exc_type, *_: object) -> None:
            await self.limiter.release(success=(exc_type is None and self.success))

    def slot(self) -> AdaptiveLimiter._CM:
        """Context manager. Set .success = True before exiting to mark the
        run as successful (otherwise treated as failure)."""
        return AdaptiveLimiter._CM(self)
