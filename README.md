mortimint - fresh breath for couchbase postmortems

mortimint flattens cbcollectinfo logs into a grep-friendlier format.

Especially, mortimint produces output with date/time-stamps on each line.

To build...

    $ go get github.com/couchbaselabs/mortimint

Usage example...

    $ cd myTempDir
    
    $ curl ...                          # Download cbcollect-info zip's.
    
    $ unzip *.zip                       # Unzip them.
    
    $ mortimint * | grep curr_items     # Grep away.

The output should have date/time-stamps, so you can use more of your
favorite cmd-line tools for more analysis and correlations.

For example, pipe output to sort to have all events from multiple log
files ordered by date/time...

    $ mortimint ~/tmp/CBSE-1313/cbcollect* | grep "  2016" | sort

NOTE: output format might change!  And, cmd-line params/flags might change.