const row = (i) => ({
  id: `id-${i}`,
  database: "db",
  schema: "public",
  name: `table_${i}`,
  size: i * 1024,
  text: `public.table_${i} ${i} bytes`,
});

function heap() {
  if (global.gc) global.gc();
  return process.memoryUsage().heapUsed;
}

const before = heap();
const page = Array.from({ length: 100 }, (_, i) => row(i));
const pageHeap = heap() - before;
const mid = heap();
const all = Array.from({ length: 10_000 }, (_, i) => row(i));
const allHeap = heap() - mid;
const limit = 64 * 1024 * 1024;
console.log(
  JSON.stringify(
    {
      node: process.version,
      platform: process.platform,
      page_rows: page.length,
      page_heap_bytes: pageHeap,
      rows_10000_heap_bytes: allHeap,
      limit_bytes: limit,
      virtualize: allHeap > limit,
    },
    null,
    2,
  ),
);
if (all.length !== 10_000) process.exit(1);
