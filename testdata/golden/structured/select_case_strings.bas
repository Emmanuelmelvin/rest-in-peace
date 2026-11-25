FOR EACH cmd IN ["start", "pause", "stop", "quit", "other"]
  SELECT CASE cmd
    CASE "start"
      PRINT "GO"
    CASE "stop", "quit"
      PRINT "HALT"
  END SELECT
NEXT
PRINT "END"
