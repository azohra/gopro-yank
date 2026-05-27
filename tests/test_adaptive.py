"""Tests for the AdaptiveLimiter — the only piece with non-trivial logic
that doesn't depend on a live GoPro API."""

import asyncio

import pytest

from gopro_yank.adaptive import AdaptiveLimiter


@pytest.mark.asyncio
async def test_grow_after_success_streak():
    lim = AdaptiveLimiter(initial=2, floor=1, ceiling=8, grow_after=3)
    # 3 successes -> target bumps to 3
    for _ in range(3):
        await lim.acquire()
        await lim.release(success=True)
    assert lim.stats().target == 3


@pytest.mark.asyncio
async def test_shrink_on_failure():
    lim = AdaptiveLimiter(initial=5, floor=1, ceiling=10, grow_after=8)
    await lim.acquire()
    await lim.release(success=False)
    assert lim.stats().target == 4


@pytest.mark.asyncio
async def test_floor_respected():
    lim = AdaptiveLimiter(initial=2, floor=1, ceiling=10, grow_after=8)
    for _ in range(5):
        await lim.acquire()
        await lim.release(success=False)
    assert lim.stats().target == 1


@pytest.mark.asyncio
async def test_ceiling_respected():
    lim = AdaptiveLimiter(initial=2, floor=1, ceiling=4, grow_after=1)
    for _ in range(20):
        await lim.acquire()
        await lim.release(success=True)
    assert lim.stats().target == 4


@pytest.mark.asyncio
async def test_slot_context_manager_success():
    lim = AdaptiveLimiter(initial=2, floor=1, ceiling=8, grow_after=2)
    async with lim.slot() as slot:
        slot.success = True
    async with lim.slot() as slot:
        slot.success = True
    assert lim.stats().target == 3


@pytest.mark.asyncio
async def test_slot_context_manager_failure_default():
    """If slot.success is never set, default counts as failure."""
    lim = AdaptiveLimiter(initial=4, floor=1, ceiling=8, grow_after=8)
    async with lim.slot():
        pass  # don't set slot.success
    assert lim.stats().target == 3
    assert lim.stats().failures == 1


@pytest.mark.asyncio
async def test_concurrent_inflight_capped_at_target():
    """Inflight should never exceed target, even under contention."""
    lim = AdaptiveLimiter(initial=3, floor=1, ceiling=3, grow_after=100)
    observed_max = 0

    async def task():
        nonlocal observed_max
        await lim.acquire()
        observed_max = max(observed_max, lim.stats().inflight)
        await asyncio.sleep(0.01)
        await lim.release(success=True)

    await asyncio.gather(*[task() for _ in range(20)])
    assert observed_max <= 3
