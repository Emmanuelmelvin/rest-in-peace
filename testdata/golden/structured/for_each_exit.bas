items = ["a", "b", "stop", "c"]
FOR EACH item IN items
  IF item = "stop" THEN EXIT FOR
  PRINT item
NEXT
PRINT "DONE"
