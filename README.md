mortimint - flatten cbcollect-info log files into a common, grep'able format.

Especially, there should be date/time-stamps on each line.

For example...

    $ cd myTempDir
    
    $ curl ... # Download several cbcollect-info zip files.
    
    $ unzip *.zip # Unzip them
    
    $ ./mortimint * | grep curr_items

The output should have date/time-stamps, which you can use more of
your favorite cmd-line tools to correlate.

For example, pipe things to sort, to get the logs sorted by date/time...

    $ ./mortimint ~/tmp/CBSE-1313/cbcollect* | grep "  2016" | sort

NOTE: output format might change!  And, cmd-line params/flags might change.