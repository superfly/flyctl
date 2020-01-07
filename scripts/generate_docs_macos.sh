echo "Running doc/main.go"
go run doc/main.go

echo "Cleaning up output"
sed -i "" -e 's/```/~~~/g' out/*.md
if [ "$1" ]
    then
        echo "rsync to $1"
        rsync out/ $1 --delete -r -v
fi