' Tier 1: Concurrency & Timers (SLEEP, SPAWN, PARALLEL)
SHARED DIM doneCount AS INTEGER = 0

SUB Work(n)
  SLEEP(10)
  doneCount = doneCount + n
END SUB

DIM a AS INTEGER = 0
DIM b AS INTEGER = 0

PARALLEL
  a = 10 + 5
  b = 20 + 7
END PARALLEL

PRINT a
PRINT b

SPAWN Work(3)
SPAWN Work(4)
SLEEP(80)
PRINT doneCount

TRY
  PARALLEL
    PRINT "ok"
    THROW ERROR("boom")
  END PARALLEL
CATCH e
  PRINT e.message
END TRY
